package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	APIUrl              string
	APIKey              string
	ClusterID           string
	LogLevel            int
	PprofPort           int
	PollIntervalSeconds int
}

var cfg *Config

// Get configuration bound to environment variables.
func Get() Config {
	if cfg != nil {
		return *cfg
	}

	_ = viper.BindEnv("loglevel", "LOG_LEVEL")

	_ = viper.BindEnv("apikey", "API_KEY")
	_ = viper.BindEnv("apiurl", "API_URL")
	_ = viper.BindEnv("clusterid", "CLUSTER_ID")

	_ = viper.BindEnv("pollintervalseconds", "POLL_INTERVAL_SECONDS")
	_ = viper.BindEnv("pprofport", "PPROF_PORT")

	cfg = &Config{}
	if err := viper.Unmarshal(&cfg); err != nil {
		panic(fmt.Errorf("parsing configuration: %v", err))
	}

	return *cfg
}
