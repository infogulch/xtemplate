#!/usr/bin/env bash

# Set up xtemplate server and cleanup
echo "Running xtemplate..."
pushd `dirname "$(readlink -f "$0")"` > /dev/null                # cd to the directory where this script is
go run ../cmd --log -4 --context-path context -minify > xtemplate.log &  # run xtemplate cmd in the background
PID=$!                                                           # grab the pid
exit() {
    sleep 0.1s       # wait for stdout to flush
    pkill -P $PID    # kill the process
    popd > /dev/null # cd back to wherever the script was invoked from
}
trap exit EXIT   # register exit handler
until grep -q -i 'msg=serving' xtemplate.log  # wait for the server to start
do
    sleep 0.1
done
echo ""

# Run tests
hurl --continue-on-error --test --report-html report tests/*.hurl
