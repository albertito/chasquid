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

	if len(c.SmtpAddress) == 0 {
		c.SmtpAddress = append(c.SmtpAddress, "systemd")
	}
	if len(c.SubmissionAddress) == 0 {
		c.SubmissionAddress = append(c.SubmissionAddress, "systemd")
	}

	if c.MailDeliveryAgentBin == "" {
		c.MailDeliveryAgentBin = "procmail"
	}
	if len(c.MailDeliveryAgentArgs) == 0 {
		c.MailDeliveryAgentArgs = append(c.MailDeliveryAgentArgs,
			"-f", "%from%", "-d", "%to_user%")
	}

	if c.DataDir == "" {
		c.DataDir = "/var/lib/chasquid"
	}

	logConfig(c)
	return c, nil
}

func logConfig(c *Config) {
	glog.Infof("Configuration:")
	glog.Infof("  Hostname: %q", c.Hostname)
	glog.Infof("  Max data size (MB): %d", c.MaxDataSizeMb)
	glog.Infof("  SMTP Addresses: %v", c.SmtpAddress)
	glog.Infof("  Submission Addresses: %v", c.SubmissionAddress)
	glog.Infof("  Monitoring address: %s", c.MonitoringAddress)
	glog.Infof("  MDA: %s %v", c.MailDeliveryAgentBin, c.MailDeliveryAgentArgs)
	glog.Infof("  Data directory: %s", c.DataDir)
	glog.Infof("  Suffix separators: %s", c.SuffixSeparators)
	glog.Infof("  Drop characters: %s", c.DropCharacters)
}
