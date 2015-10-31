// Package config implements the chasquid configuration.
package config

// Generate the config protobuf.
//go:generate protoc --go_out=. config.proto

import (
	"io/ioutil"
	"os"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
)

// Load the config from the given file.
func Load(path string) (*Config, error) {
	c := &Config{}

	buf, err := ioutil.ReadFile(path)
	if err != nil {
		glog.Errorf("Failed to read config at %q", path)
		glog.Errorf("  (%v)", err)
		return nil, err
	}

	err = proto.UnmarshalText(string(buf), c)
	if err != nil {
		glog.Errorf("Error parsing config: %v", err)
		return nil, err
	}

	// Fill in defaults for anything that's missing.

	if c.Hostname == "" {
		c.Hostname, err = os.Hostname()
		if err != nil {
			glog.Errorf("Could not get hostname: %v", err)
			return nil, err
		}
	}

	if c.MaxDataSizeMb == 0 {
		c.MaxDataSizeMb = 50
	}

	if len(c.Address) == 0 {
		c.Address = append(c.Address, "systemd")
	}

	logConfig(c)
	return c, nil
}

func logConfig(c *Config) {
	glog.Infof("Configuration:")
	glog.Infof("  Hostname: %q", c.Hostname)
	glog.Infof("  Max data size (MB): %d", c.MaxDataSizeMb)
	glog.Infof("  Addresses: %v", c.Address)
	glog.Infof("  Monitoring address: %s", c.MonitoringAddress)
}
