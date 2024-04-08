package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/vngcloud/cloud-provider-vngcloud/pkg/ingress/config"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/ingress/controller"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/version"
	"k8s.io/component-base/cli"
	"k8s.io/klog/v2"
)

var (
	cfgFile string
	isDebug bool
	conf    config.Config
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "ingress-vngcloud",
	Short: "Ingress controller for VngCloud",
	Long:  `Ingress controller for VngCloud`,

	Run: func(cmd *cobra.Command, args []string) {
		osIngress := controller.NewController(conf)
		osIngress.Start()

		sigterm := make(chan os.Signal, 1)
		signal.Notify(sigterm, syscall.SIGTERM)
		signal.Notify(sigterm, syscall.SIGINT)
		<-sigterm
	},
	Version: version.Version,
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	code := cli.Run(rootCmd)
	os.Exit(code)
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFormatter(&log.TextFormatter{
		ForceColors:            true,
		FullTimestamp:          true,
		DisableLevelTruncation: true,
		PadLevelText:           true,
	})

	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.ingress_openstack.yaml)")
	rootCmd.PersistentFlags().BoolVar(&isDebug, "debug", false, "Print more detailed information.")

	klog.InitFlags(nil)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".ingress_openstack" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".ingress_openstack")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		log.WithFields(log.Fields{"error": err}).Fatal("Failed to read config file")
	}

	log.WithFields(log.Fields{"file": viper.ConfigFileUsed()}).Info("Using config file")

	if err := viper.Unmarshal(&conf); err != nil {
		log.WithFields(log.Fields{"error": err}).Fatal("Unable to decode the configuration")
	}

	if isDebug {
		log.SetLevel(log.DebugLevel)
	}
}
