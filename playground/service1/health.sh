#!/bin/bash
if curl -sf http://localhost:4512 >/dev/null 2>&1; then
    echo "Health OK"
else
    echo "Health FAIL"
fi
