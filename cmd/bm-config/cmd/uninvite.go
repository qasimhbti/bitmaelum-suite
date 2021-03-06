package cmd

import (
	"fmt"
	"github.com/bitmaelum/bitmaelum-suite/internal/container"
	"github.com/bitmaelum/bitmaelum-suite/pkg/address"
	"github.com/spf13/cobra"
)

// uninviteCmd represents the uninvite command
var uninviteCmd = &cobra.Command{
	Use:   "uninvite",
	Short: "Removes the invitation for the given address",
	Long:  `Removes the invitation for the given address. This address cannot register on your server until you invite them again.`,
	Run: func(cmd *cobra.Command, args []string) {
		s, _ := cmd.Flags().GetString("address")

		addr, err := address.New(s)
		if err != nil {
			fmt.Printf("incorrect address specified")
			return
		}

		inviteRepo := container.GetInviteRepo()
		err = inviteRepo.Remove(addr.Hash())
		if err != nil {
			fmt.Printf("error while uninviting address")
		}

		fmt.Printf("'%s' has been removed.\n", addr.String())
	},
}

func init() {
	rootCmd.AddCommand(uninviteCmd)

	uninviteCmd.Flags().String("address", "", "Address to uninvite")
}
