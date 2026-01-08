#!/bin/bash
if curl -sf http://localhost:4512 >/dev/null 2>&1; then
    echo '{"icon": "🟢", "message": "All systems operational"}' > .devir-status
    echo "Health OK"
else
    echo '{"icon": "🔴", "color": "red", "message": "Service down!"}' > .devir-status
    echo "Health FAIL"
fi
