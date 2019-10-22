# Package configuration
PROJECT = metadata-retrieval
COMMANDS = examples/cmd

PKG_OS = windows darwin linux

# Including ci Makefile
CI_REPOSITORY ?= https://github.com/src-d/ci.git
CI_BRANCH ?= v1
CI_PATH ?= .ci
MAKEFILE := $(CI_PATH)/Makefile.main
TEST_RACE ?= true
$(MAKEFILE):
	git clone --quiet --depth 1 -b $(CI_BRANCH) $(CI_REPOSITORY) $(CI_PATH);
-include $(MAKEFILE)

migration: get-go-bindata
	go-bindata -pkg database -prefix database/migrations -o database/migration.go database/migrations

# better to use `tools.go` as described in
# https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
# but go-bindata doesn't support go modules yet and `go get` fails
get-go-bindata:
	GO111MODULE=off go get -u github.com/go-bindata/go-bindata/...
