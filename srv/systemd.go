package srv

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"srv.exe.dev/db/dbgen"
)

// Config with defaults, overridable via environment variables
var (
	systemdDir    = getEnvDefault("NEWS_APP_SYSTEMD_DIR", "/etc/systemd/system")
	jobRunnerPath = getEnvDefault("NEWS_APP_JOB_RUNNER", "/home/exedev/news-app/news-app")
	jobRunnerArgs = getEnvDefault("NEWS_APP_JOB_RUNNER_ARGS", "run-job") // subcommand
	workingDir    = getEnvDefault("NEWS_APP_WORKING_DIR", "/home/exedev/news-app")
	runAsUser     = getEnvDefault("NEWS_APP_RUN_USER", "exedev")
)

func getEnvDefault(key, defaultVal string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return defaultVal
}

func createSystemdTimer(job dbgen.Job) error {
	serviceName := fmt.Sprintf("news-job-%d", job.ID)
	
	// Create service file
	serviceContent := fmt.Sprintf(`[Unit]
Description=News Job %d: %s
After=network.target

[Service]
Type=oneshot
ExecStart=%s %s %d
User=%s
WorkingDirectory=%s
RuntimeMaxSec=1800

[Install]
WantedBy=multi-user.target
`, job.ID, job.Name, jobRunnerPath, jobRunnerArgs, job.ID, runAsUser, workingDir)
	
	servicePath := filepath.Join(systemdDir, serviceName+".service")
	if err := writeFileWithSudo(servicePath, serviceContent); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}
	
	// Create timer file (only for recurring jobs)
	if job.IsOneTime == 0 {
		timerContent := fmt.Sprintf(`[Unit]
Description=Timer for News Job %d: %s

[Timer]
OnCalendar=%s
Persistent=true

[Install]
WantedBy=timers.target
`, job.ID, job.Name, frequencyToCalendar(job.Frequency))
		
		timerPath := filepath.Join(systemdDir, serviceName+".timer")
		if err := writeFileWithSudo(timerPath, timerContent); err != nil {
			return fmt.Errorf("write timer file: %w", err)
		}
		
		// Enable and start timer
		exec.Command("sudo", "systemctl", "daemon-reload").Run()
		exec.Command("sudo", "systemctl", "enable", serviceName+".timer").Run()
		exec.Command("sudo", "systemctl", "start", serviceName+".timer").Run()
	} else {
		// For one-time jobs, just reload and run immediately (in background)
		exec.Command("sudo", "systemctl", "daemon-reload").Run()
		// Use --no-block to avoid waiting for job completion
		exec.Command("sudo", "systemctl", "start", "--no-block", serviceName+".service").Run()
	}
	
	return nil
}

func updateSystemdTimer(job dbgen.Job) error {
	serviceName := fmt.Sprintf("news-job-%d", job.ID)
	
	if job.IsActive == 0 {
		// Job is inactive - stop timer but don't stop running service
		exec.Command("sudo", "systemctl", "stop", serviceName+".timer").Run()
		exec.Command("sudo", "systemctl", "disable", serviceName+".timer").Run()
		return nil
	}
	
	// Job is active - update the timer/service files without stopping running processes
	// Just stop and disable the timer (not the service) before updating
	exec.Command("sudo", "systemctl", "stop", serviceName+".timer").Run()
	exec.Command("sudo", "systemctl", "disable", serviceName+".timer").Run()
	
	// Create/update the service and timer files
	return createSystemdTimer(job)
}

func removeSystemdTimer(jobID int64) {
	serviceName := fmt.Sprintf("news-job-%d", jobID)
	
	// Stop the timer but don't stop the service - let running jobs complete
	exec.Command("sudo", "systemctl", "stop", serviceName+".timer").Run()
	exec.Command("sudo", "systemctl", "disable", serviceName+".timer").Run()
	// Note: We intentionally don't stop the service here to allow running jobs to complete
	
	os.Remove(filepath.Join(systemdDir, serviceName+".service"))
	os.Remove(filepath.Join(systemdDir, serviceName+".timer"))
	
	exec.Command("sudo", "systemctl", "daemon-reload").Run()
}

func frequencyToCalendar(freq string) string {
	switch freq {
	case "hourly":
		return "*-*-* *:00:00"
	case "6hours":
		return "*-*-* 00/6:00:00"
	case "daily":
		return "*-*-* 06:00:00"
	case "weekly":
		return "Mon *-*-* 06:00:00"
	default:
		return "*-*-* 06:00:00"
	}
}

func writeFileWithSudo(path, content string) error {
	// Write to temp file then move with sudo
	tmpFile, err := os.CreateTemp("", "systemd-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	
	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()
	
	cmd := exec.Command("sudo", "cp", tmpPath, path)
	return cmd.Run()
}

func runJobDirectly(db *sql.DB, jobID int64) {
	cmd := exec.Command(jobRunnerPath, fmt.Sprintf("%d", jobID))
	cmd.Run()
}
