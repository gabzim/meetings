# Filters binary

This binary reads events from stdin, filters them and emits the passing events to stdout.

For instance, if you want to blink lights for events marked with tomato color, you can do:

`connect | filters -c tomato -b 30s | lights -r 'office'`

The command above connects to the api's websocket, filters events with the tomato color on them, and emits them 30s `-b`efore they start. Then the `lights` binary picks it up and blinks the lights. 
