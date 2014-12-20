#!/bin/bash
echo "okay"
screen -dmS MineCraft java -Xmx1024M -Xms1024M -jar minecraft_server.jar nogui
while true; do
	uname -a | nc -zv localhost 25567
	OUT=$?
	if [ $OUT -eq 0 ];then
		exit 0
	else
		echo "Not yet started, trying again..."
	fi
	sleep 1
done
