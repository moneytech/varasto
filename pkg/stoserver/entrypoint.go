package stoserver

import (
	"fmt"
	"github.com/function61/gokit/logex"
	"github.com/function61/gokit/ossignal"
	"github.com/function61/gokit/stopper"
	"github.com/function61/gokit/systemdinstaller"
	"github.com/function61/varasto/pkg/stoserver/stodbimportexport"
	"github.com/spf13/cobra"
	"log"
	"os"
)

func Entrypoint() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Starts the server component",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			rootLogger := logex.StandardLogger()

			workers := stopper.NewManager()
			go func(logger *log.Logger) {
				logex.Levels(logger).Info.Printf(
					"Got %s; stopping",
					<-ossignal.InterruptOrTerminate())

				workers.StopAllWorkersAndWait()
			}(logex.Prefix("main", rootLogger))

			panicIfError(runServer(rootLogger, workers.Stopper()))
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "dbimport",
		Short: "Imports database",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			scf, err := readServerConfigFile()
			if err != nil {
				panic(err)
			}

			if err := stodbimportexport.Import(os.Stdin, scf.DbLocation); err != nil {
				panic(err)
			}
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Installs systemd unit file to make Varasto start on system boot",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			hints, err := systemdinstaller.InstallSystemdServiceFile(
				"varasto",
				[]string{"server"},
				"Varasto server")

			if err != nil {
				panic(err)
			} else {
				fmt.Println(hints)
			}
		},
	})

	return cmd
}