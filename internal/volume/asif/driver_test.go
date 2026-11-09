//go:build darwin

package asif

import (
	"os"
	"path"
	"testing"

	"github.com/rtgnx/vz/internal/volume/types"
	"github.com/stretchr/testify/assert"
)

func TestASIFCreate(t *testing.T) {
	tmpDir, err := os.MkdirTemp(
		os.TempDir(), `vm-asif-create-test-*`,
	)

	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmpDir)

	driver := New(tmpDir)

	if assert.NoError(t, driver.Create(`test`, 1)) {
		fp := path.Join(tmpDir, `test.asif`)
		assert.FileExists(t, fp)

		assert.ErrorIs(
			t, driver.Create(`test`, 1),
			types.ErrVolumeExists,
		)
	}

}

func TestASIFDelete(t *testing.T) {
	tmpDir, err := os.MkdirTemp(
		os.TempDir(), `vm-asif-create-test-*`,
	)

	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmpDir)

	driver := New(tmpDir)

	if assert.NoError(t, driver.Create(`test`, 1)) {
		fp := path.Join(tmpDir, `test.asif`)

		if !assert.FileExists(t, fp) {
			t.FailNow()
		}

		if assert.NoError(t, driver.Delete(`test`)) {
			assert.NoFileExists(t, fp)
		}
	}

}

func TestASIFList(t *testing.T) {
	tmpDir, err := os.MkdirTemp(
		os.TempDir(), `vm-asif-create-test-*`,
	)

	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmpDir)

	driver := New(tmpDir)

	if assert.NoError(t, driver.Create(`test`, 1)) {
		fp := path.Join(tmpDir, `test.asif`)

		if !assert.FileExists(t, fp) {
			t.FailNow()
		}

		volumes, err := driver.List()

		if assert.NoError(t, err) {
			if assert.Len(t, volumes, 1) {
				assert.Equal(t, uint64(0), volumes[0].SizeGB)
				assert.Equal(t, `test`, volumes[0].Name)
				assert.Equal(t, `ASIF`, volumes[0].Format)
			}
		}
	}

}

func TestASIFOpen(t *testing.T) {
	tmpDir, err := os.MkdirTemp(
		os.TempDir(), `vm-asif-create-test-*`,
	)

	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmpDir)

	driver := New(tmpDir)

	if assert.NoError(t, driver.Create(`test`, 1)) {
		fp := path.Join(tmpDir, `test.asif`)

		if !assert.FileExists(t, fp) {
			t.FailNow()
		}

		volume, err := driver.Open(`test`)

		if assert.NoError(t, err) {
			assert.NoError(t, volume.Close())
		}
	}

}
