# How to gen proto

* Insure that go dir added to PATH, e.g. 
```
export PATH=$PATH:~/go/bin
```
* Download opentelemetry-proto project
```
git clone https://github.com/open-telemetry/opentelemetry-proto.git
```
* Run command
```
protoc -I/Users/artem/dev/agerime/observability/opentelemetry/opentelemetry-proto -I. --go_out=. --go-grpc_out=. logs.proto
```