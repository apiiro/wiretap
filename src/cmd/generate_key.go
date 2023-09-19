package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var generateKeyCmd = &cobra.Command{
	Use:   "genkey",
	Short: "Generate key pair",
	Long:  `Generate wireguard private public key`,
	Run: func(cmd *cobra.Command, args []string) {
		run()
	},
}

func init() {
	rootCmd.AddCommand(generateKeyCmd)
}

func run() {
	key, err := wgtypes.GeneratePrivateKey()

	if err != nil {
		log.Fatalln("Error generating key", err)
	}

	fmt.Println("Private Key:", key.String())
	fmt.Println("Public Key:", key.PublicKey().String())
}
