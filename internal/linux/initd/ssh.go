//go:build linux

package initd

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"

	gliderssh "github.com/gliderlabs/ssh"

	gossh "golang.org/x/crypto/ssh"
)

func ParseSSHKeys(keys ...string) ([]gossh.PublicKey, error) {
	parsed := make([]gossh.PublicKey, 0, len(keys))

	for i, raw := range keys {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.ContainsAny(raw, "\r\n") {
			return nil, fmt.Errorf("SSH key %d: expected one public key line", i+1)
		}

		key, _, options, _, err := gossh.ParseAuthorizedKey([]byte(raw))
		if err != nil {
			return nil, fmt.Errorf("SSH key %d: %w", i+1, err)
		}
		if len(options) > 0 {
			return nil, fmt.Errorf("SSH key %d: authorized_keys options are unsupported", i+1)
		}

		parsed = append(parsed, key)
	}

	return parsed, nil
}

func ServeSSH(ctx context.Context, validKeys []gossh.PublicKey) error {
	allowed := make(map[string]struct{}, len(validKeys))
	for _, key := range validKeys {
		if key != nil {
			allowed[string(key.Marshal())] = struct{}{}
		}
	}

	// Example host key. Persist this between VM boots in a real server,
	// or SSH clients will see a different host identity each time.
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	hostSigner, err := gossh.NewSignerFromKey(privateKey)
	if err != nil {
		return err
	}

	server := &gliderssh.Server{
		Addr: ":22",
		PublicKeyHandler: func(_ gliderssh.Context, key gliderssh.PublicKey) bool {
			_, ok := allowed[string(key.Marshal())]
			return ok
		},
		Handler: func(session gliderssh.Session) {
			if len(session.Command()) != 0 {
				_ = session.Exit(126) // This endpoint only accepts an interactive shell.
				return
			}

			req, windowChanges, ok := session.Pty()
			if !ok {
				_, _ = io.WriteString(session, "request a PTY with ssh -t\n")
				_ = session.Exit(1)
				return
			}

			cmd := exec.CommandContext(session.Context(), "/bin/sh")
			cmd.Dir = "/"
			cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: "/new_root"}

			ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
				Rows: uint16(req.Window.Height),
				Cols: uint16(req.Window.Width),
			})
			if err != nil {
				_, _ = fmt.Fprintln(session, err)
				_ = session.Exit(1)
				return
			}
			defer ptmx.Close()

			go func() {
				for window := range windowChanges {
					_ = pty.Setsize(ptmx, &pty.Winsize{
						Rows: uint16(window.Height),
						Cols: uint16(window.Width),
					})
				}
			}()
			go func() {
				_, _ = io.Copy(ptmx, session)
			}()

			_, _ = io.Copy(session, ptmx)
			if err := cmd.Wait(); err != nil {
				_ = session.Exit(1)
				return
			}
			_ = session.Exit(0)
		},
	}
	server.AddHostKey(hostSigner)

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
		}
		return nil
	case err := <-serveErr:
		return err
	}
}
