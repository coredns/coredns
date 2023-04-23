package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/coredns/coredns/plugin/atlas"
	"github.com/coredns/coredns/plugin/atlas/ent"
	"github.com/coredns/coredns/plugin/atlas/utils"
	"github.com/spf13/cobra"
)

var (
	zoneImportOptions *ZoneImportOptions
)

type ZoneImportOptions struct {
	dsn        string
	file       string
	domain     string
	bulk       bool
	directory  string
	fileNaming string
}

func NewZoneImportOptions() *ZoneImportOptions {
	return &ZoneImportOptions{}
}

func init() {
	zoneImportOptions = NewZoneImportOptions()

	zoneImportCmd := &cobra.Command{
		Use:          "zoneImport",
		Short:        "import a zone from a zone file or bulk import a directory",
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := zoneImportOptions.Complete(c, args); err != nil {
				return err
			}
			if err := zoneImportOptions.Validate(); err != nil {
				return err
			}
			if err := zoneImportOptions.Run(); err != nil {
				return err
			}
			return nil
		},
	}

	importZoneFlags := zoneImportCmd.Flags()
	importZoneFlags.StringVarP(&zoneImportOptions.domain, "domain", "d", "", "domain to import to database")
	importZoneFlags.StringVarP(&zoneImportOptions.file, "file", "f", "", "zone file name to import")
	importZoneFlags.BoolVarP(&zoneImportOptions.bulk, "bulk", "b", false, "enable bulk directory import")
	importZoneFlags.StringVarP(&zoneImportOptions.directory, "dir", "", "", "bulk import directory")
	importZoneFlags.StringVarP(&zoneImportOptions.fileNaming, "tpl", "", "pri.{.Domain}", "file template name")

	rootCmd.AddCommand(zoneImportCmd)
}

func (o *ZoneImportOptions) Complete(cmd *cobra.Command, args []string) (err error) {
	o.dsn = cfg.db.DSN

	return nil
}

func (o *ZoneImportOptions) Validate() (err error) {
	if len(o.file) == 0 && !o.bulk {
		return fmt.Errorf("no file found")
	}

	if len(o.domain) == 0 && !o.bulk {
		return fmt.Errorf("expected domain")
	}

	if o.bulk && len(o.directory) == 0 {
		return fmt.Errorf("expected directory")
	}

	return nil
}

func (o *ZoneImportOptions) Run() error {
	var client *ent.Client
	var err error

	client, err = atlas.OpenAtlasDB(o.dsn)
	if err != nil {
		return err
	}
	defer client.Close()

	// this code migrates the database automatically!
	err = client.Schema.Create(context.Background())
	if err != nil {
		return err
	}

	f, err := os.Open(o.file)
	if err != nil {
		return err
	}

	return utils.ImportZone(context.Background(), client, f, o.domain, o.file)
}
