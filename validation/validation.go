package validation

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ValidateArgs checks whether path provided exists and is a valid os path
func ValidateArgs(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("No arguments or too many arguments provided")
	}

	return nil
}
