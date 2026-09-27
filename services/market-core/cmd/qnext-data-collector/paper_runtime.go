package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func superviseSynPlusPaperRuntime(ctx context.Context, storageRoot, marketSymbol string) {
	if !strings.EqualFold(strings.TrimSpace(marketSymbol), "NIFTY") {
		return
	}

	moduleRoot := resolveStrategyLabRoot()
	if moduleRoot == "" {
		log.Printf("SYN+ paper runtime disabled: Strategy Lab package not found")
		return
	}
	pythonBin := env("QNEXT_PYTHON_BIN", "python3")
	if _, err := exec.LookPath(pythonBin); err != nil {
		log.Printf("SYN+ paper runtime disabled: %s not available: %v", pythonBin, err)
		return
	}

	addr := env("QNEXT_SYN_PLUS_PAPER_ADDR", "127.0.0.1:18082")
	pythonPath := moduleRoot
	if current := strings.TrimSpace(os.Getenv("PYTHONPATH")); current != "" {
		pythonPath += string(os.PathListSeparator) + current
	}

	for ctx.Err() == nil {
		cmd := exec.CommandContext(ctx, pythonBin, "-m", "qnext_strategy_lab.syn_plus_runtime")
		cmd.Env = append(
			os.Environ(),
			"PYTHONPATH="+pythonPath,
			"QNEXT_STORAGE_ROOT="+storageRoot,
			"QNEXT_SYN_PLUS_PAPER_ADDR="+addr,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		log.Printf("SYN+ paper runtime starting addr=%s strategy_lab=%s", addr, moduleRoot)
		err := cmd.Run()
		if ctx.Err() != nil {
			return
		}
		log.Printf("SYN+ paper runtime exited: %v; restarting", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func resolveStrategyLabRoot() string {
	candidates := make([]string, 0, 4)
	if explicit := strings.TrimSpace(os.Getenv("QNEXT_STRATEGY_LAB_ROOT")); explicit != "" {
		candidates = append(candidates, explicit)
	}
	if executable, err := os.Executable(); err == nil {
		privateRoot := filepath.Dir(filepath.Dir(executable))
		candidates = append(candidates, filepath.Join(privateRoot, "strategy-lab"))
	}
	candidates = append(candidates, "../strategy-lab", "services/strategy-lab")

	for _, candidate := range candidates {
		module := filepath.Join(candidate, "qnext_strategy_lab", "syn_plus_runtime.py")
		if info, err := os.Stat(module); err == nil && !info.IsDir() {
			absolute, absErr := filepath.Abs(candidate)
			if absErr == nil {
				return absolute
			}
			return candidate
		}
	}
	return ""
}
