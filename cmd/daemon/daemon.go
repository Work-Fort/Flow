// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Work-Fort/Flow/internal/config"
	flowDaemon "github.com/Work-Fort/Flow/internal/daemon"
	"github.com/Work-Fort/Flow/internal/infra"
)

// NewCmd returns the daemon cobra command.
func NewCmd() *cobra.Command {
	var bind string
	var port int
	var db string
	var passportURL string
	var serviceToken string
	var pylonURL string
	var webhookBaseURL string

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the Flow daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("bind") {
				bind = viper.GetString("bind")
			}
			if !cmd.Flags().Changed("port") {
				port = viper.GetInt("port")
			}
			if !cmd.Flags().Changed("db") {
				db = viper.GetString("db")
			}
			if !cmd.Flags().Changed("passport-url") {
				passportURL = viper.GetString("passport-url")
			}
			if !cmd.Flags().Changed("service-token") {
				serviceToken = viper.GetString("service-token")
			}
			if !cmd.Flags().Changed("pylon-url") {
				pylonURL = viper.GetString("pylon.url")
			}
			if !cmd.Flags().Changed("webhook-base-url") {
				webhookBaseURL = viper.GetString("webhook-base-url")
			}

			hiveSvcName := viper.GetString("pylon.services.hive")
			sharkfinSvcName := viper.GetString("pylon.services.sharkfin")

			return run(bind, port, db, passportURL, serviceToken, pylonURL, webhookBaseURL, hiveSvcName, sharkfinSvcName)
		},
	}

	cmd.Flags().StringVar(&bind, "bind", "127.0.0.1", "Bind address")
	cmd.Flags().IntVar(&port, "port", 17200, "Listen port")
	cmd.Flags().StringVar(&db, "db", "", "Database file path (empty = default location)")
	cmd.Flags().StringVar(&passportURL, "passport-url", "", "Passport auth service URL")
	cmd.Flags().StringVar(&serviceToken, "service-token", "", "Service identity token (Passport API key)")
	cmd.Flags().StringVar(&pylonURL, "pylon-url", "", "Pylon service registry URL")
	cmd.Flags().StringVar(&webhookBaseURL, "webhook-base-url", "", "Flow's externally reachable base URL for webhook callbacks")

	return cmd
}

func run(bind string, port int, db, passportURL, serviceToken, pylonURL, webhookBaseURL, hiveSvcName, sharkfinSvcName string) error {
	health := flowDaemon.NewHealthService()

	dsn := db
	if dsn == "" {
		dsn = filepath.Join(config.GlobalPaths.StateDir, "flow.db")
	}

	store, err := infra.Open(dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer store.Close()

	health.RegisterPeriodicCheck("db", func(ctx context.Context) flowDaemon.CheckResult {
		if err := store.Ping(ctx); err != nil {
			return flowDaemon.CheckResult{Severity: flowDaemon.SeverityError, Message: err.Error()}
		}
		return flowDaemon.CheckResult{Severity: flowDaemon.SeverityOK}
	})

	srv := flowDaemon.NewServer(flowDaemon.ServerConfig{
		Bind:         bind,
		Port:         port,
		PassportURL:  passportURL,
		ServiceToken: serviceToken,
		PylonURL:     pylonURL,
		PylonServices: flowDaemon.PylonServices{
			Hive:     hiveSvcName,
			Sharkfin: sharkfinSvcName,
		},
		WebhookBaseURL: webhookBaseURL,
		Health:         health,
		Store:          store,
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		if err := flowDaemon.ListenAndServe(srv); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Start periodic health checks
	healthCtx, healthCancel := context.WithCancel(context.Background())
	defer healthCancel()
	go health.StartPeriodic(healthCtx, 30*time.Second)

	select {
	case sig := <-sigCh:
		log.Info("received signal, shutting down", "signal", sig)
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("http shutdown", "err", err)
	}

	return nil
}
