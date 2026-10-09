VERSION=1

# Go images build from the repository root so they use the local log-sender and protos.
all:
	docker build --file log-analysis/Dockerfile --tag docker-log-analysis:${VERSION} .
	cd ./log-ui && docker build --tag docker-log-ui:${VERSION} .
	docker build --file log-receiver/Dockerfile --tag docker-log-receiver:${VERSION} .
	docker network create troubleshooting_network

save:
	docker save -o docker-log-analysis-${VERSION}.tar docker-log-analysis:${VERSION}
	docker save -o docker-log-ui-${VERSION}.tar docker-log-ui:${VERSION}
	docker save -o docker-log-receiver-${VERSION}.tar docker-log-receiver:${VERSION}

upload:
	#scp docker-log-analysis-${VERSION}.tar ${SERVER}:${SERVER_PATH}
	#scp docker-log-ui-${VERSION}.tar ${SERVER}:${SERVER_PATH}
	scp docker-log-receiver-${VERSION}.tar ${SERVER}:${SERVER_PATH}
