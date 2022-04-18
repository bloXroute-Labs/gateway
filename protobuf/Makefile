IMAGE_NAME=bloxroute/bdn-protobuf:v3.19.3
LOCAL_IMAGE_NAME=bx-proto-gen

.PHONY: genproto
.SILENT: genproto
genproto:
	docker run -v $(CURDIR):/go/protobuf $(IMAGE_NAME) \
		protoc --go_out=. --go_opt=paths=source_relative  --go-grpc_out=. --go-grpc_opt=paths=source_relative gateway.proto

GENERATE_CMD=docker run -v $(CURDIR):/go/protobuf $(LOCAL_IMAGE_NAME) \
	protoc --go_out=. --go_opt=paths=source_relative  --go-grpc_out=. --go-grpc_opt=paths=source_relative gateway.proto

.PHONY: genproto-local
.SILENT: genproto-local
genproto-local:
	docker image inspect $(LOCAL_IMAGE_NAME) > /dev/null 2>&1 && $(GENERATE_CMD) || (docker build --platform linux/x86_64 . -t $(LOCAL_IMAGE_NAME) && $(GENERATE_CMD))