#!/bin/bash
echo "Service 1 starting..."
counter=0
while true; do
    counter=$((counter + 1))
    echo "[INFO] Service 1 - tick $counter"
    if [ $((counter % 5)) -eq 0 ]; then
        echo "[WARN] Service 1 - warning at tick $counter"
    fi
    if [ $((counter % 10)) -eq 0 ]; then
        echo "[ERROR] Service 1 - error at tick $counter" >&2
    fi
    sleep 2
done
