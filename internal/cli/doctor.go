package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var (
		asJSON bool
		fix    bool
	)
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Compare recorded workspaces with git and the filesystem",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newContainer(cmd)
			doctor, err := c.doctor()
			if err != nil {
				return err
			}
			issues, err := doctor.Diagnose()
			if err != nil {
				return err
			}
			if fix {
				repairer, err := c.repairer()
				if err != nil {
					return err
				}
				if err := repairer.Repair(issues); err != nil {
					return err
				}
				// Re-diagnose so the report shows what the repair left.
				if issues, err = doctor.Diagnose(); err != nil {
					return err
				}
			}

			presenter := presenterFor(cmd, asJSON)
			if err := presenter.Issues(issues); err != nil {
				return err
			}
			// A machine-readable run puts the findings in the payload; only
			// the human-facing run turns them into a failing exit status.
			if presenter.MachineReadable() || len(issues) == 0 {
				return nil
			}
			if fix {
				return fmt.Errorf("doctor still reports %d issue(s)", len(issues))
			}
			return fmt.Errorf("doctor found %d issue(s)", len(issues))
		},
	}
	cmd.Flags().BoolVarP(&asJSON, "json", "j", false, "print JSON")
	cmd.Flags().BoolVar(&fix, "fix", false, "apply conservative repairs")
	return cmd
}
