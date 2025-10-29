package migrations

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type BrokenFS struct {
}

func (b BrokenFS) Open(name string) (fs.File, error) {
	return nil, fs.ErrNotExist
}

func TestPostgresMigration(t *testing.T) {
	t.Run("creates migration with embedded filesystem", func(t *testing.T) {
		migration := NewPostgresMigration()

		assert.NotNil(t, migration, "Expected migration to be non-nil")
	})

	t.Run("returns correct source type", func(t *testing.T) {
		migration := NewPostgresMigration()
		require.NotNil(t, migration, "Expected migration to be non-nil")

		sourceType := migration.GetSourceType()

		assert.Equal(t, "iofs", sourceType, "Expected source type to be 'iofs'")
	})

	t.Run("successfully creates source driver from embedded files", func(t *testing.T) {
		migration := NewPostgresMigration()
		require.NotNil(t, migration, "Expected migration to be non-nil")

		driver, err := migration.GetSourceDriver()

		require.NoError(t, err, "Expected no error when creating source driver")
		assert.NotNil(t, driver, "Expected driver to be non-nil")
	})

	t.Run("handles nil filesystem gracefully", func(t *testing.T) {
		migration := &PostgresMigration{
			fs: BrokenFS{},
		}

		driver, err := migration.GetSourceDriver()

		assert.Error(t, err, "Expected error when filesystem is nil")
		assert.Nil(t, driver, "Expected driver to be nil on error")
		assert.Contains(t, err.Error(), "failed to create migration source", "Error message should indicate migration source failure")
	})
}
