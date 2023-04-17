package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	_ "github.com/coredns/coredns/plugin/atlas/ent/runtime"
	_ "github.com/lib/pq"
)

const CliName = "atlas"

var cfgFile string

type db struct {
	Port     int
	Name     string
	Hostname string
	Username string
	Password string
	SSL      bool
}

func (d db) GetDSN() string {
	return fmt.Sprintf("host=%s port=%v user=%s dbname=%s password=%s sslmode=%s", d.Hostname, d.Port, d.Username, d.Name, d.Password, d.SSLMode())
}

func (d db) SSLMode() string {
	if d.SSL {
		return "require"
	}
	return "disable"
}

type config struct {
	db
}

var (
	cfg    = &config{}
	Logger *zap.Logger
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "atlas",
	Short: "atlas for isp providers",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/atlas.yaml)")
	var err error

	config := zap.NewProductionConfig()
	config.EncoderConfig.EncodeTime = zapcore.EpochNanosTimeEncoder
	config.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	config.EncoderConfig.EncodeName = zapcore.FullNameEncoder

	Logger, err = config.Build()
	if err != nil {
		panic(err)
	}
	defer Logger.Sync()
	// Logger = Logger.WithOptions(zap.AddCallerSkip(1))

	//rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.atlas.yaml)")
	pflags := rootCmd.PersistentFlags()
	pflags.StringVarP(&cfg.db.Hostname, "db.hostname", "", "localhost", "database hostname")
	pflags.StringVarP(&cfg.db.Name, "db.name", "", "postgres", "database name")
	pflags.IntVarP(&cfg.db.Port, "db.port", "", 5432, "database port")
	pflags.StringVarP(&cfg.db.Username, "db.user", "", "", "database user")
	pflags.StringVarP(&cfg.db.Password, "db.password", "", "", "database password")
	pflags.BoolVarP(&cfg.db.SSL, "db.ssl", "", false, "database ssl mode")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".cli" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName("atlas")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
