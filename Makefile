all:
	cd ./log-analysis && docker build --tag docker-log-analysis:1 .
	cd ./log-ui && docker build --tag docker-log-ui:1 .
	cd ./log-receiver && docker build --tag docker-log-receiver:1 .
