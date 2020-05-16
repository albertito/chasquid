// Package config implements the chasquid configuration.
package config

// Generate the config protobuf.
//go:generate protoc --go_out=. --go_opt=paths=source_relative config.proto

import (
	"fmt"
	"io/ioutil"
	"os"

	"blitiri.com.ar/go/log"

	"google.golang.org/protobuf/encoding/prototext"
)

// Load the config from the given file.
func Load(path string) (*Config, error) {
	c := &Config{}

	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config at %q: %v", path, err)
	}

	err = prototext.Unmarshal(buf, c)
	if err != nil {
		return nil, fmt.Errorf("parsing config: %v", err)
	}

	// Fill in defaults for anything that's missing.

	if c.Hostname == "" {
		c.Hostname, err = os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("could not get hostname: %v", err)
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
	if len(c.SubmissionOverTlsAddress) == 0 {
		c.SubmissionOverTlsAddress = append(c.SubmissionOverTlsAddress, "systemd")
	}

	if c.MailDeliveryAgentBin == "" {
		c.MailDeliveryAgentBin = "maildrop"
	}
	if len(c.MailDeliveryAgentArgs) == 0 {
		c.MailDeliveryAgentArgs = append(c.MailDeliveryAgentArgs,
			"-f", "%from%", "-d", "%to_user%")
	}

	if c.DataDir == "" {
		c.DataDir = "/var/lib/chasquid"
	}

	if c.SuffixSeparators == "" {
		c.SuffixSeparators = "+"
	}

	if c.DropCharacters == "" {
		c.DropCharacters = "."
	}

	if c.MailLogPath == "" {
		c.MailLogPath = "<syslog>"
	}

	return c, nil
}

// LogConfig logs the given configuration, in a human-friendly way.
func LogConfig(c *Config) {
	log.Infof("Configuration:")
	log.Infof("  Hostname: %q", c.Hostname)
	log.Infof("  Max data size (MB): %d", c.MaxDataSizeMb)
	log.Infof("  SMTP Addresses: %v", c.SmtpAddress)
	log.Infof("  Submission Addresses: %v", c.SubmissionAddress)
	log.Infof("  Submission+TLS Addresses: %v", c.SubmissionOverTlsAddress)
	log.Infof("  Monitoring address: %s", c.MonitoringAddress)
	log.Infof("  MDA: %s %v", c.MailDeliveryAgentBin, c.MailDeliveryAgentArgs)
	log.Infof("  Data directory: %s", c.DataDir)
	log.Infof("  Suffix separators: %s", c.SuffixSeparators)
	log.Infof("  Drop characters: %s", c.DropCharacters)
	log.Infof("  Mail log: %s", c.MailLogPath)
	log.Infof("  Dovecot auth: %v (%q, %q)",
		c.DovecotAuth, c.DovecotUserdbPath, c.DovecotClientPath)
}
