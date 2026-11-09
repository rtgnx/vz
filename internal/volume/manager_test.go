//go:build darwin

package volume

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rtgnx/vz/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestNewManager(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), `volume-manager-tests-*`)

	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmpDir)

	var (
		cfgFileMissing = filepath.Join(tmpDir, `volumes.yml`)
	)

	m, err := NewManager(cfgFileMissing)
	if assert.NoError(t, err) {
		if assert.NoError(t, m.Close()) {
			assert.FileExists(t, cfgFileMissing)
		}
	}
}

func TestCreate(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), `volume-manager-tests-*`)

	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmpDir)

	var (
		cfgFileMissing = filepath.Join(tmpDir, `volumes.yml`)
		testVol        = config.Volume{
			Name:   "testvol",
			Size:   1,
			Format: `asif`,
		}
	)

	m, err := NewManager(cfgFileMissing)

	if assert.NoError(t, err) {
		if assert.NoError(t, m.Create(t.Context(), testVol)) {
			volumePath := filepath.Join(
				filepath.Dir(cfgFileMissing),
				fmt.Sprintf("%s.%s", testVol.Name, testVol.Format),
			)

			assert.FileExists(t, volumePath)
			if assert.NoError(t, m.Close()) {
				volumes, err := config.Read[[]config.Volume](cfgFileMissing)
				if assert.NoError(t, err) {
					if assert.Len(t, *volumes, 1) {
						assert.Equal(t, testVol.Size, (*volumes)[0].Size)
						assert.Equal(t, testVol.Name, (*volumes)[0].Name)
						assert.Equal(t, testVol.Format, (*volumes)[0].Format)
					}
				}
			}
		}
	}
}

func TestList(t *testing.T) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), `volume-manager-tests-*`)

	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmpDir)

	var (
		cfgFileMissing = filepath.Join(tmpDir, `volumes.yml`)
		testVolumes    = map[string]config.Volume{
			"test-1": config.Volume{
				Name:   "test-1",
				Format: `asif`,
				Size:   1,
			},
			"test-2": config.Volume{
				Name:   "test-2",
				Format: `asif`,
				Size:   1,
			},
		}
	)

	m, err := NewManager(cfgFileMissing)

	if !assert.NoError(t, err) {
		t.FailNow()
	}

	for _, tv := range testVolumes {
		assert.NoError(t, m.Create(t.Context(), tv))
	}

	list, err := m.List(t.Context())

	if assert.NoError(t, err) {
		for _, v := range list {
			tv, ok := testVolumes[v.Name]
			if !assert.True(t, ok, "listed volume not present in testVolumes") {
				continue
			}

			assert.Equal(t, tv.Name, v.Name)
			assert.Equal(t, tv.Format, strings.ToLower(v.Format))
		}
	}

}

func TestDelete(t *testing.T) {

	tmpDir, err := os.MkdirTemp(os.TempDir(), `volume-manager-tests-*`)

	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmpDir)

	var (
		cfgFileMissing = filepath.Join(tmpDir, `volumes.yml`)
		testVol        = config.Volume{
			Name:   "testvol",
			Size:   1,
			Format: `asif`,
		}
	)

	m, err := NewManager(cfgFileMissing)

	if assert.NoError(t, err) {
		if assert.NoError(t, m.Create(t.Context(), testVol)) {
			volumePath := filepath.Join(
				filepath.Dir(cfgFileMissing),
				fmt.Sprintf("%s.%s", testVol.Name, testVol.Format),
			)

			assert.FileExists(t, volumePath)
			if assert.NoError(t, m.Delete(t.Context(), testVol.Name)) {
				assert.NoFileExists(t, volumePath)
				if assert.NoError(t, m.Close()) {
					volumes, err := config.Read[[]config.Volume](cfgFileMissing)
					if assert.NoError(t, err) {
						assert.Len(t, *volumes, 0)
					}
				}
			}

		}
	}
}
