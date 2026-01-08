#!/bin/bash
echo "Service 2 starting..."
counter=0
while true; do
    counter=$((counter + 1))
    echo "[INFO] Service 2 - processing item $counter"
    if [ $((counter % 3)) -eq 0 ]; then
        echo "[DEBUG] Service 2 - debug info at $counter"
    fi
    sleep 3
done
