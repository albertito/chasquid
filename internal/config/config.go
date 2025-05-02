// Package config implements the chasquid configuration.
package config

// Generate the config protobuf.
//go:generate protoc --go_out=. --go_opt=paths=source_relative --experimental_allow_proto3_optional config.proto

import (
	"fmt"
	"os"
	"time"

	"blitiri.com.ar/go/log"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

var defaultConfig = &Config{
	MaxDataSizeMb: 50,

	SmtpAddress:              []string{"systemd"},
	SubmissionAddress:        []string{"systemd"},
	SubmissionOverTlsAddress: []string{"systemd"},

	MailDeliveryAgentBin:  "maildrop",
	MailDeliveryAgentArgs: []string{"-f", "%from%", "-d", "%to_user%"},

	DataDir: "/var/lib/chasquid",

	SuffixSeparators: proto.String("+"),
	DropCharacters:   proto.String("."),

	MailLogPath: "<syslog>",

	MaxQueueItems:   200,
	GiveUpSendAfter: "20h",
}

// Load the config from the given file, with the given overrides.
func Load(path, overrides string) (*Config, error) {
	// Start with a copy of the default config.
	c := proto.Clone(defaultConfig).(*Config)

	// Load from the path.
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config at %q: %v", path, err)
	}

	fromFile := &Config{}
	err = prototext.Unmarshal(buf, fromFile)
	if err != nil {
		return nil, fmt.Errorf("parsing config: %v", err)
	}
	override(c, fromFile)

	// Handle command line overrides.
	fromOverrides := &Config{}
	err = prototext.Unmarshal([]byte(overrides), fromOverrides)
	if err != nil {
		return nil, fmt.Errorf("parsing override: %v", err)
	}
	override(c, fromOverrides)

	// Handle hostname separate, because if it is set, we don't need to call
	// os.Hostname which can fail.
	if c.Hostname == "" {
		c.Hostname, err = os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("could not get hostname: %v", err)
		}
	}

	// Validate the GiveUpSendAfter value.
	if _, err := time.ParseDuration(c.GiveUpSendAfter); err != nil {
		return nil, fmt.Errorf(
			"invalid give_up_send_after value %q: %v", c.GiveUpSendAfter, err)
	}

	return c, nil
}

// Override fields in `c` that are set in `o`. We can't use proto.Merge
// because the semantics would not be convenient for overriding.
func override(c, o *Config) {
	if o.Hostname != "" {
		c.Hostname = o.Hostname
	}
	if o.MaxDataSizeMb > 0 {
		c.MaxDataSizeMb = o.MaxDataSizeMb
	}
	if len(o.SmtpAddress) > 0 {
		c.SmtpAddress = o.SmtpAddress
	}
	if len(o.SubmissionAddress) > 0 {
		c.SubmissionAddress = o.SubmissionAddress
	}
	if len(o.SubmissionOverTlsAddress) > 0 {
		c.SubmissionOverTlsAddress = o.SubmissionOverTlsAddress
	}
	if o.MonitoringAddress != "" {
		c.MonitoringAddress = o.MonitoringAddress
	}

	if o.MailDeliveryAgentBin != "" {
		c.MailDeliveryAgentBin = o.MailDeliveryAgentBin
	}
	if len(o.MailDeliveryAgentArgs) > 0 {
		c.MailDeliveryAgentArgs = o.MailDeliveryAgentArgs
	}

	if o.DataDir != "" {
		c.DataDir = o.DataDir
	}

	if o.SuffixSeparators != nil {
		c.SuffixSeparators = o.SuffixSeparators
	}
	if o.DropCharacters != nil {
		c.DropCharacters = o.DropCharacters
	}
	if o.MailLogPath != "" {
		c.MailLogPath = o.MailLogPath
	}

	if o.DovecotAuth {
		c.DovecotAuth = true
	}
	if o.DovecotUserdbPath != "" {
		c.DovecotUserdbPath = o.DovecotUserdbPath
	}
	if o.DovecotClientPath != "" {
		c.DovecotClientPath = o.DovecotClientPath
	}

	if o.HaproxyIncoming {
		c.HaproxyIncoming = true
	}

	if o.MaxQueueItems > 0 {
		c.MaxQueueItems = o.MaxQueueItems
	}
	if o.GiveUpSendAfter != "" {
		c.GiveUpSendAfter = o.GiveUpSendAfter
	}
}

// LogConfig logs the given configuration, in a human-friendly way.
func LogConfig(c *Config) {
	log.Infof("Configuration:")
	log.Infof("  Hostname: %q", c.Hostname)
	log.Infof("  Max data size (MB): %d", c.MaxDataSizeMb)
	log.Infof("  SMTP Addresses: %q", c.SmtpAddress)
	log.Infof("  Submission Addresses: %q", c.SubmissionAddress)
	log.Infof("  Submission+TLS Addresses: %q", c.SubmissionOverTlsAddress)
	log.Infof("  Monitoring address: %q", c.MonitoringAddress)
	log.Infof("  MDA: %q %q", c.MailDeliveryAgentBin, c.MailDeliveryAgentArgs)
	log.Infof("  Data directory: %q", c.DataDir)
	if c.SuffixSeparators == nil {
		log.Infof("  Suffix separators: nil")
	} else {
		log.Infof("  Suffix separators: %q", *c.SuffixSeparators)
	}
	if c.DropCharacters == nil {
		log.Infof("  Drop characters: nil")
	} else {
		log.Infof("  Drop characters: %q", *c.DropCharacters)
	}
	log.Infof("  Mail log: %q", c.MailLogPath)
	log.Infof("  Dovecot auth: %v (%q, %q)",
		c.DovecotAuth, c.DovecotUserdbPath, c.DovecotClientPath)
	log.Infof("  HAProxy incoming: %v", c.HaproxyIncoming)
	log.Infof("  Max queue items: %d", c.MaxQueueItems)
	log.Infof("  Give up send after: %s", c.GiveUpSendAfterDuration())
}

func (c *Config) GiveUpSendAfterDuration() time.Duration {
	// We validate the string value at config load time, so we know it is well
	// formed.
	d, _ := time.ParseDuration(c.GiveUpSendAfter)
	return d
}
