package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/bitmaelum/bitmaelum-suite/internal/container"
	"github.com/bitmaelum/bitmaelum-suite/internal/parse"
	"github.com/bitmaelum/bitmaelum-suite/pkg/address"
	"github.com/spf13/cobra"
	"time"
)

type jsonOut map[string]interface{}

// inviteCmd represents the invite command
var inviteCmd = &cobra.Command{
	Use:   "invite",
	Short: "Invite a new user onto your server",
	Long: `This command will generate an invitation token that must be used for registering an account on your 
server. Only the specified address can register the account`,
	Run: func(cmd *cobra.Command, args []string) {
		s, _ := cmd.Flags().GetString("address")
		d, _ := cmd.Flags().GetString("duration")
		asJSON, _ := cmd.Flags().GetBool("json")

		addr, err := address.New(s)
		if err != nil {
			outError("incorrect address specified", asJSON)
			return
		}

		duration, err := parse.ValidDuration(d)
		if err != nil {
			outError("incorrect duration specified", asJSON)
			return
		}

		inviteRepo := container.GetInviteRepo()
		token, err := inviteRepo.Get(addr.Hash())
		if err == nil {
			msg := fmt.Sprintf("'%s' already allowed to register with token: %s\n", addr.String(), token)
			outError(msg, asJSON)
			return
		}

		token, err = inviteRepo.Create(addr.Hash(), duration)
		if err != nil {
			msg := fmt.Sprintf("error while inviting address: %s", err)
			outError(msg, asJSON)
			return
		}

		if asJSON {
			output := jsonOut{
				"address":     addr.String(),
				"token":       token,
				"valid_until": time.Now().Add(duration),
			}
			out, _ := json.Marshal(output)
			fmt.Printf("%s", out)
		} else {
			fmt.Printf("'%s' is allowed to register on our server until %s.\n", addr.String(), time.Now().Add(duration).Format(time.RFC822))
			fmt.Printf("The invitation token is: %s\n", token)
		}
	},
}

func outError(msg string, asJSON bool) {
	if !asJSON {
		fmt.Print(msg)
		return
	}

	out, _ := json.Marshal(jsonOut{"error": msg})
	fmt.Printf("%s", out)
}

func init() {
	rootCmd.AddCommand(inviteCmd)

	inviteCmd.Flags().String("address", "", "Address to register")
	inviteCmd.Flags().String("duration", "30", "NUmber of days (or duration like 1w2d3h4m6s) allowed for registration")
	inviteCmd.Flags().Bool("json", false, "Return JSON response when set")

	_ = inviteCmd.MarkFlagRequired("address")
}
