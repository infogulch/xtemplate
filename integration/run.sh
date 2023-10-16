#!/usr/bin/env bash

# Set up xtemplate server and cleanup
echo "Running xtemplate..."
pushd `dirname "$(readlink -f "$0")"` > /dev/null    # cd to the directory where this script is
go run ../bin -log -4 > xtemplate.log &              # exec go run in the background
PID=$!                                               # grab the pid
exit() {         # define exit handler
    sleep 0.1s   # wait for stdout to flush
    kill $PID    # kill the process
    popd         # cd back to wherever the script was invoked from
}
trap exit EXIT   # register exit handler
until grep -q -i 'msg=serving' xtemplate.log  # wait for the server to start
do
    sleep 0.1
done
echo ""

# Run tests
hurl --continue-on-error --test --report-html report *.hurl
