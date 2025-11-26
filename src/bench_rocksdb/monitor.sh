#!/bin/bash

CMD="$@"

$CMD &
PID=$!
echo "PID: $PID"
# sleep 1

cat /proc/$PID/io > before.txt

while kill -0 $PID 2>/dev/null; do
    if [ -d "/proc/$PID" ]; then
        cat /proc/$PID/io 
	sleep 0.1
	echo "----------------------------------"
    else
        echo "Process already exited, using last snapshot"
    fi
done


echo "==================== DONE ===================="
