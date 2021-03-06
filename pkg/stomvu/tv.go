package stomvu

import (
	"os"
	"path/filepath"

	"github.com/function61/varasto/pkg/seasonepisodedetector"
	"github.com/spf13/cobra"
)

func tvEntrypoint() *cobra.Command {
	doIt := false

	cmd := &cobra.Command{
		Use:   "tv",
		Short: "Renames TV episodes",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			plan, err := computePlan(episodeFromFilename)
			panicIfError(err)

			if doIt {
				panicIfError(executePlan(plan))
			} else {
				explainPlan(plan, os.Stdout)
			}
		},
	}

	cmd.Flags().BoolVarP(&doIt, "do", "", doIt, "Whether to execute the plan or run a dry run")

	return cmd
}

func episodeFromFilename(input string) string {
	result := seasonepisodedetector.Detect(input)
	if result == nil {
		return ""
	}

	// "S03/S03E07"
	return filepath.Join(result.SeasonDesignation(), result.String())
}
