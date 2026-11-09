//go:build darwin

package asif

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rtgnx/vz/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestVolumeSnapshot(t *testing.T) {
	tmpDir, err := os.MkdirTemp(
		os.TempDir(), `vm-asif-snapshot-test-*`,
	)

	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmpDir)

	driver := New(tmpDir)

	if !assert.NoError(t, driver.Create("test", 1)) {
		t.FailNow()
	}

	volume, err := driver.Open("test")

	if !assert.NoError(t, err) {
		t.FailNow()
	}

	vol := volume.(*Volume)
	if assert.NoError(t, vol.Snapshot(`test-snapshot`)) {
		layerFp := filepath.Join(tmpDir, `layers`, vol.manifest.Current)
		manifestFp := filepath.Join(tmpDir, `test.yml`)

		assert.FileExists(t, layerFp)
		assert.FileExists(t, manifestFp)

		manifest, err := config.Read[Manifest](manifestFp)
		if assert.NoError(t, err) {
			if assert.Len(t, manifest.Layers, 1) {
				assert.Equal(t, manifest.Current, manifest.Layers[0])
			}
		}

		snapshotUUID := vol.image.LayerUUID()

		if assert.NoError(t, vol.Close()) {
			vol, err := driver.Open(`test`)
			if assert.NoError(t, err) {
				assert.Equal(t, snapshotUUID, vol.(*Volume).image.LayerUUID())
			}
		}
	}

}
