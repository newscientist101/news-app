#!/bin/bash
DB_PATH="/home/exedev/news-app/db.sqlite3"

# Parse logs and extract job_id, timestamp, duplicate_count
journalctl -u 'news-job-*' --no-pager 2>/dev/null | \
awk '
/Running job [0-9]+ via Shelley API/ {
    match($0, /run-job.sh\[([0-9]+)\]/, pid)
    match($0, /Running job ([0-9]+)/, job)
    if (pid[1] && job[1]) {
        current_pid = pid[1]
        current_job = job[1]
        dup_count[current_pid] = 0
        job_id[current_pid] = current_job
        # Extract date parts
        month[current_pid] = $1
        day[current_pid] = $2
        time[current_pid] = $3
    }
}
/Skipped duplicate:/ {
    match($0, /run-job.sh\[([0-9]+)\]/, pid)
    if (pid[1] && dup_count[pid[1]] != "") {
        dup_count[pid[1]]++
    }
}
/Job [0-9]+ completed/ {
    match($0, /run-job.sh\[([0-9]+)\]/, pid)
    if (pid[1] && job_id[pid[1]]) {
        # Output: job_id|month|day|time|dup_count
        print job_id[pid[1]] "|" month[pid[1]] "|" day[pid[1]] "|" time[pid[1]] "|" dup_count[pid[1]]
    }
}
' | while IFS='|' read -r job_id month day time dup_count; do
    # Convert month name to number
    case $month in
        Jan) mon="01" ;; Feb) mon="02" ;; Mar) mon="03" ;; Apr) mon="04" ;;
        May) mon="05" ;; Jun) mon="06" ;; Jul) mon="07" ;; Aug) mon="08" ;;
        Sep) mon="09" ;; Oct) mon="10" ;; Nov) mon="11" ;; Dec) mon="12" ;;
    esac
    
    # Build date string for matching (assuming year 2026)
    # The started_at in DB is like: 2026-02-08 06:01:18
    # We need to match within a reasonable window (same minute)
    date_prefix="2026-${mon}-${day} ${time%:*}"  # Remove seconds for matching
    
    # Find matching job_run and update if dup_count > 0
    if [ "$dup_count" -gt 0 ]; then
        # Find the job_run that matches this job_id and approximate time
        run_id=$(sqlite3 "$DB_PATH" "SELECT id FROM job_runs WHERE job_id = $job_id AND started_at LIKE '${date_prefix}%' LIMIT 1;")
        if [ -n "$run_id" ]; then
            echo "Updating run $run_id (job $job_id at $date_prefix) with $dup_count duplicates"
            sqlite3 "$DB_PATH" "UPDATE job_runs SET duplicates_skipped = $dup_count WHERE id = $run_id;"
        fi
    fi
done

echo "Backfill complete!"
