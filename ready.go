package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v2"
)

type (
	task struct {
		Command   string `yaml:"command"`
		Directory string `yaml:"directory"`
		Name      string `yaml:"name"`
	}
	config struct {
		Tasks []task `yaml:"tasks"`
	}
)

var Version string

func main() {
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		fmt.Println("\n\nReady stopped 🛑")
		os.Exit(0)
	}()

	if len(os.Args) > 1 && os.Args[1] == "init" {
		err := installHook()
		if err != nil {
			log.Fatalf("Error installing hook: %v 💥\n", err)
		}
		fmt.Println("Ready ready ✅")
		os.Exit(0)
	}

	all := flag.Bool("all", false, "Run all tasks without commit")
	version := flag.Bool("version", false, "Print Ready version")

	flag.Parse()

	if *version {
		if Version == "" {
			info, ok := debug.ReadBuildInfo()
			if !ok {
				log.Fatal("Failed to read build info 💥\n")
			}
			fmt.Printf("Ready version %v ℹ️\n", info.Main.Version)
			return
		}
		fmt.Printf("Ready version %v ℹ️\n", Version)
		return
	}

	cfg, err := newConfig().withYAML()
	if err != nil {
		log.Fatalf("Failed to get config: %v 💥\n", err)
	}

	successes := 0
	failures := 0
	start := time.Now()

	for _, t := range cfg.Tasks {
		if !*all {
			staged, err := exec.Command("git", "diff", "--name-only", "--cached", "--diff-filter=AM").CombinedOutput()
			if err != nil {
				log.Fatalf("Error determining files with changes: %v 💥\n", err)
			}
			if len(staged) == 0 {
				continue
			}
			if t.Directory != "" && !strings.Contains(string(staged), t.Directory) {
				continue
			}
		}

		fmt.Printf("Running task %s... ⏳ ", t.Name)

		output, err := runTask(t)
		if err != nil {
			fmt.Printf("Failure ❌\n\n%v\n", err)
			failures++
			continue
		}

		successes++

		if output == "" {
			fmt.Printf("Success ✅\n\n")
		} else {
			fmt.Printf("Success ✅\n\n%v\n", output)
		}
	}

	if successes == 0 && failures == 0 {
		fmt.Println("Nothing to do 💤")
		return
	}

	if failures > 0 {
		if failures == 1 {
			fmt.Printf("Got 1 failure. Please fix it and try again ⚠️ \n\n")
		} else {
			fmt.Printf("Got %d failures. Please fix them and try again ⚠️ \n\n", failures)
		}
		os.Exit(1)
	}

	fmt.Printf("%d tasks completed successfully in %v ✨\n\n", successes, time.Since(start).Round(time.Millisecond))
}

func installHook() error {
	hook := filepath.FromSlash("./.git/hooks/pre-commit")
	_, err := os.Open(hook)
	if err == nil {
		fmt.Println("A pre-commit hook already exists ℹ️  Do you want to overwrite it? [yes/no]")
		res := ""
		fmt.Fscan(os.Stdin, &res)
		if res != "yes" {
			fmt.Println("Ready stopped 🛑")
			os.Exit(0)
		}
	}

	content := []byte(`
#!/bin/sh
# Hook created by Ready https://github.com/lewislbr/ready

initial_state=$(git diff --name-only)

ready

exit_status=$?
if [ $exit_status -ne 0 ]; then
	exit $exit_status
fi

latest_state=$(git diff --name-only)
if [[ $latest_state != $initial_state ]]; then
	echo "Some files have been modified by the hook. Please handle them and commit again 🔧"
	exit 1
fi

exit 0
`)
	err = os.WriteFile(hook, content, 0o755)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}

	return nil
}

func newConfig() *config {
	return &config{}
}

func (c *config) withYAML() (*config, error) {
	path, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("determining current path: %w", err)
	}

	file := filepath.Join(strings.TrimSuffix(string(path), "\n"), "ready.yaml")
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	err = yaml.Unmarshal([]byte(data), &c)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling data: %w", err)
	}

	return c, nil
}

func runTask(t task) (string, error) {
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", t.Command)
	} else {
		cmd = exec.Command("/bin/sh", "-c", t.Command)
	}

	if t.Directory != "" {
		cmd.Dir = t.Directory
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		if string(output) == "" {
			return "", err
		}
		return "", errors.New(string(output))
	}

	return string(output), nil
}
