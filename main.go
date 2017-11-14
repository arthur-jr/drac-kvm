// -*- go -*-

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/Unknwon/goconfig"
	"github.com/howeyc/gopass"
	"github.com/ogier/pflag"
)

// CLI flags
var _host = pflag.StringP("host", "h", "some.hostname.com", "The DRAC host (or IP)")
var _username = pflag.StringP("username", "u", "", "The DRAC username")
var _password = pflag.BoolP("password", "p", false, "Prompt for password (optional, will use 'calvin' if not present)")
var _version = pflag.IntP("version", "v", -1, "iDRAC version (6, 7 or 8)")
var _delay = pflag.IntP("delay", "d", 10, "Number of seconds to delay for javaws to start up & read jnlp before deleting it")
var _javaws = pflag.StringP("javaws", "j", DefaultJavaPath(), "The path to javaws binary")
var _wait = pflag.BoolP("wait", "w", false, "Wait for java console process end")

const (
	// DefaultUsername is the default username on Dell iDRAC
	DefaultUsername = "root"
	// DefaultPassword is the default password on Dell iDRAC
	DefaultPassword = "calvin"
)

func promptPassword() string {
	fmt.Print("Password: ")
	password, _ := gopass.GetPasswd()
	return string(password)
}

func get_javaws_args(wait_flag bool) string {
	var javaws_args string = "-jnlp"

	cmd := exec.Command("java", "-version")
	stderr, err := cmd.StderrPipe()

	if err != nil {
		//os.Remove(filename)
		log.Fatalf("Java not present on your system...", err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	slurp, _ := ioutil.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		log.Fatal(err)
	}

	if strings.Contains(string(slurp[:]), "1.7") ||
		strings.Contains(string(slurp[:]), "1.8") {
		if wait_flag {
			javaws_args = "-wait"
		} else {
			javaws_args = ""
		}

	}

	return javaws_args
}

func main() {
	var host string
	var username string
	var password string

	// Parse the CLI flags
	pflag.Parse()

	// Check we have access to the javaws binary
	if _, err := os.Stat(*_javaws); err != nil {
		log.Fatalf("No javaws binary found at %s", *_javaws)
	}

	// Search for existing config file
	usr, _ := user.Current()
	cfg, _ := goconfig.LoadConfigFile(usr.HomeDir + "/.drackvmrc")
	version := *_version

	// Get the default username and password from the config
	if cfg != nil {
		_, err := cfg.GetSection("defaults")
		if err == nil {
			log.Printf("Loading default username and password from configuration file")
			uservalue, uerr := cfg.GetValue("defaults", "username")
			passvalue, perr := cfg.GetValue("defaults", "password")

			if uerr == nil {
				username = uservalue
			} else {
				username = DefaultUsername
			}
			if perr == nil {
				password = passvalue
			} else {
				password = DefaultPassword
			}

		}
	}

	// Finding host in config file or using the one passed in param
	host = *_host
	hostFound := false
	if cfg != nil {
		_, err := cfg.GetSection(*_host)
		if err == nil {
			value, err := cfg.GetValue(*_host, "host")
			if err == nil {
				hostFound = true
				host = value
			} else {
				hostFound = true
				host = *_host
			}
		}
	}

	if *_username != "" {
		username = *_username
	} else {
		if cfg != nil && hostFound {
			value, err := cfg.GetValue(*_host, "username")
			if err == nil {
				username = value
			}
		}
	}

	// If password not set, prompt
	if *_password {
		password = promptPassword()
	} else {
		if cfg != nil && hostFound {
			value, err := cfg.GetValue(*_host, "password")
			if err == nil {
				password = value
			}
		}
	}
	if username == "" && password == "" {
		log.Printf("Username/Password not provided trying without them...")
	}

	drac := &DRAC{
		Host:     host,
		Username: username,
		Password: password,
		Version:  version,
	}

	// Generate a DRAC viewer JNLP
	viewer, err := drac.Viewer()
	if err != nil {
		log.Fatalf("Unable to generate DRAC viewer for %s@%s (%s)", drac.Username, drac.Host, err)
	}

	// Write out the DRAC viewer to a temporary file so that
	// we can launch it with the javaws program
	filename := os.TempDir() + string(os.PathSeparator) + "drac_" + drac.Host + ".jnlp"
	ioutil.WriteFile(filename, []byte(viewer), 0600)
	defer os.Remove(filename)

	// Launch it!
	log.Printf("Launching DRAC KVM session to %s with %s", drac.Host, filename)
	if err := exec.Command(*_javaws, get_javaws_args(*_wait), filename, "-nosecurity", "-noupdate", "-Xnofork").Run(); err != nil {
		os.Remove(filename)
		log.Fatalf("Unable to launch DRAC (%s), from file %s", err, filename)
	}

	// Give javaws a few seconds to start & read the jnlp
	time.Sleep(time.Duration(*_delay) * time.Second)
}

// EOF
