package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "chrome",
		"--no-sandbox",
		"--disable-dev-shm-usage",
		"--disable-gpu",
		"--disable-blink-features=AutomationControlled",
		"--no-first-run",
		"--disable-default-apps",
		"--window-size=1280,800",
		"--headless=new",
		"https://www.facebook.com/login",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Error: %v\nOutput: %s", err, out)
	}

	fmt.Printf("Output: %s\n", out)
}
