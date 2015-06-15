package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"
)

const (
	name          = "expire-files"
	version       = "0.0.1"
	defaultExpiry = 8 * 60 * 60 // 8 hours.
	graceSeconds  = 60 * 60
)

var (
	configFile string
	sessionDir string
	debugOn    bool
	dryrunOn   bool
	batchSize  = 1000
)

// Package-level init.
func init() {
	// Setup cli flags.
	flag.StringVar(&configFile, "c", "/etc/php5/apache2/php.ini", "php config that contains the session.gc_maxlifetime variable")
	flag.StringVar(&sessionDir, "d", "/var/php/", "php file sessions directory")
	flag.BoolVar(&debugOn, "debug", false, "turn on debugging")
	flag.BoolVar(&dryrunOn, "dryrun", false, "turn on dry-run mode")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s v%s\nUsage: %s [arguments] \n", name, version, name)
		flag.PrintDefaults()
	}
}

// Main program.
func main() {
	// Parse CLI flags.
	flag.Parse()

	// Init logger.
	initLogger()

	// Determine if we're the only process running.
	// err := gosingleton.UniqueBinary(os.Getpid())
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// Determine of the php config file that contains the session.gc_maxlifetime variable exists.
	if _, err := os.Stat(configFile); err != nil {
		log.Fatal(err)
	}

	// Read the session.gc_maxlifetime variable.
	sessionExpiry := readSessionExpiry(configFile)

	// Open the php sessions directory.
	d, err := os.Open(sessionDir)
	if err != nil {
		log.Fatal(err)
	}

	// Read files from the directory in batches to limit resource usage and protect against a directory with a large number of files.
	readMore := true
	now := time.Now()
	duration, err := time.ParseDuration(fmt.Sprintf("-%ds", sessionExpiry+graceSeconds))
	if err != nil {
		log.Fatal(err)
	}

	count := 0
	for readMore {
		fs, err := d.Readdir(batchSize)
		if err != nil {
			if err == io.EOF {
				break
			}

			log.Fatal(err)
		}

		for _, f := range fs {
			if f.IsDir() {
				continue
			}

			if now.Add(duration).After(f.ModTime()) {
				if !dryrunOn {
					os.Remove(fmt.Sprintf("%s%s", d.Name(), f.Name()))
				}
				count++
				debug("Deleted: %s%s", d.Name(), f.Name())
			}
		}
	}

	log.Printf("Expired %d session files from %s", count, sessionDir)
}

// Init the logger.
func initLogger() {
	if debugOn {
		log.SetFlags(log.LstdFlags | log.Llongfile)
		log.Print("Debugging enabled")
	} else {
		log.SetFlags(0) // Disable all embellishments because this will go to syslog via cron/logger.
	}
}

// Debug prints debug messages if the debug mode is on.
func debug(format string, args ...interface{}) {
	if debugOn {
		log.Printf("DEBUG: "+format, args...)
	}
}

// Read the session.gc_maxlifetime variable from the php config file.
func readSessionExpiry(c string) int {
	var (
		expiry int
		length int
	)

	f, err := os.Open(c)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		length = len(s.Text())
		if length < 27 { // Exit early if our string is not long enough.
			continue
		}

		if s.Text()[0:25] == "session.gc_maxlifetime = " {
			expiry, err = strconv.Atoi(s.Text()[25:length])
			if err != nil {
				return defaultExpiry
			}
		}
	}

	if err := s.Err(); err != nil {
		log.Fatal(err)
	}

	return expiry
}
