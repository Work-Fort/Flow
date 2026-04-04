// SPDX-License-Identifier: GPL-2.0-only
package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Work-Fort/Flow/internal/infra"
	"github.com/Work-Fort/Flow/internal/transfer"
)

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "Admin commands"}
	cmd.AddCommand(newSeedCmd())
	return cmd
}

func newSeedCmd() *cobra.Command {
	var db, file string
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Load a workflow template from a JSON file",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := infra.Open(db)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer store.Close()
			t, err := transfer.ImportTemplate(context.Background(), store, file, nil)
			if err != nil {
				return fmt.Errorf("import template: %w", err)
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(t)
		},
	}
	cmd.Flags().StringVar(&db, "db", "", "Database path (required)")
	cmd.Flags().StringVar(&file, "file", "", "Path to workflow JSON template (required)")
	cmd.MarkFlagRequired("db")
	cmd.MarkFlagRequired("file")
	return cmd
}
