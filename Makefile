# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

.PHONY: autonity embed-autonity-contract embed-oracle-contract android ios autonity-cross evm all test clean lint mock-gen test-fast

NPMBIN= $(shell npm bin)
BINDIR = ./build/bin
GO ?= latest
LATEST_COMMIT ?= $(shell git log -n 1 develop --pretty=format:"%H")
ifeq ($(LATEST_COMMIT),)
LATEST_COMMIT := $(shell git log -n 1 HEAD~1 --pretty=format:"%H")
endif
SOLC_VERSION = 0.8.12
SOLC_BINARY = $(BINDIR)/solc_static_linux_v$(SOLC_VERSION)
GOBINDATA_VERSION = 3.23.0
GOBINDATA_BINARY = $(BINDIR)/go-bindata
ABIGEN_BINARY = $(BINDIR)/abigen

AUTONITY_CONTRACT_BASE_DIR = ./autonity/solidity
AUTONITY_CONSENSUS_TEST_DIR = ./consensus/test
AUTONITY_NEW_E2E_TEST_DIR = ./e2e_test
AUTONITY_CONTRACT_DIR = $(AUTONITY_CONTRACT_BASE_DIR)/contracts
AUTONITY_CONTRACT_TEST_DIR = $(AUTONITY_CONTRACT_BASE_DIR)/test
AUTONITY = Autonity
GENERATED_CONTRACT_DIR = ./common/acdefault/generated
GENERATED_RAW_ABI = $(GENERATED_CONTRACT_DIR)/Autonity.abi
GENERATED_ABI = $(GENERATED_CONTRACT_DIR)/abi.go
GENERATED_BYTECODE = $(GENERATED_CONTRACT_DIR)/bytecode.go
GENERATED_UPGRADE_ABI = $(GENERATED_CONTRACT_DIR)/abi_upgrade.go
GENERATED_UPGRADE_BYTECODE = $(GENERATED_CONTRACT_DIR)/bytecode_upgrade.go
GENERATED_GO_BINDINGS = $(AUTONITY_CONSENSUS_TEST_DIR)/ac_go_binding_gen_test.go
GENERATED_E2E_TEST_GO_BINDINGS = $(AUTONITY_NEW_E2E_TEST_DIR)/autonity_contracts_bindings.go


ORACLE = Oracle
GENERATED_ORACLE_CONTRACT_DIR = ./common/ocdefault/generated
# DOCKER_SUDO is set to either the empty string or "sudo" and is used to
# control whether docker is executed with sudo or not. If the user is root or
# the user is in the docker group then this will be set to the empty string,
# otherwise it will be set to "sudo".
#
# We make use of posix's short-circuit evaluation of "or" expressions
# (https://pubs.opengroup.org/onlinepubs/009695399/utilities/xcu_chap02.html#tag_02_09_03)
# where the second part of the expression is only executed if the first part
# evaluates to false. We first check to see if the user id is 0 since this
# indicates that the user is root. If not we then use 'id -nG $USER' to list
# all groups that the user is part of and then grep for the word docker in the
# output, if grep matches the word it returns the successful error code. If not
# we then echo "sudo".
DOCKER_SUDO = $(shell [ `id -u` -eq 0 ] || id -nG $(USER) | grep "\<docker\>" > /dev/null || echo sudo )

# Builds the docker image and checks that we can run the autonity binary inside
# it.
build-docker-image:
	@$(DOCKER_SUDO) docker build -t autonity .
	@$(DOCKER_SUDO) docker run --rm autonity -h > /dev/null

autonity:
	mkdir -p $(BINDIR)
	go build -o $(BINDIR)/autonity ./cmd/autonity
	@echo "Done building."
	@echo "Run \"$(BINDIR)/autonity\" to launch autonity."

generate-go-bindings:
	@echo Generating $(GENERATED_GO_BINDINGS)
	$(ABIGEN_BINARY)  --pkg test --solc $(SOLC_BINARY) --sol $(AUTONITY_CONTRACT_DIR)/$(AUTONITY).sol --out $(GENERATED_GO_BINDINGS)
	$(ABIGEN_BINARY)  --pkg test --solc $(SOLC_BINARY) --sol $(AUTONITY_CONTRACT_DIR)/$(AUTONITY).sol --out $(GENERATED_E2E_TEST_GO_BINDINGS)

# Builds Autonity without contract compilation, useful with alpine containers not supporting
# glibc for solc.
autonity-docker:
	mkdir -p $(BINDIR)
	go build -o $(BINDIR)/autonity ./cmd/autonity
	@echo "Done building."
	@echo "Run \"$(BINDIR)/autonity\" to launch autonity."

define gen-contract
	@mkdir -p $(1)
	$(SOLC_BINARY) --overwrite --abi --bin -o $(1) $(AUTONITY_CONTRACT_DIR)/$(2).sol

	@echo Generating $(1)/bytecode.go
	@echo 'package generated' > $(1)/bytecode.go
	@echo -n 'const Bytecode = "' >> $(1)/bytecode.go
	@cat  $(1)/$(2).bin >> $(1)/bytecode.go
	@echo '"' >> $(1)/bytecode.go
	@gofmt -s -w $(1)/bytecode.go

	@echo Generating $(1)/abi.go
	@echo 'package generated' > $(1)/abi.go
	@echo -n 'const Abi = `' >> $(1)/abi.go
	@cat  $(1)/$(2).abi | json_pp  >> $(1)/abi.go
	@echo '`' >> $(1)/abi.go
	@gofmt -s -w $(1)/abi.go
endef

# Genreates go source files containing the oracle contract bytecode and abi.
embed-oracle-contract: $(SOLC_BINARY) $(GOBINDATA_BINARY)
	@$(call gen-contract,$(GENERATED_ORACLE_CONTRACT_DIR),$(ORACLE))
	# update 4byte selector for clef
	build/generate_4bytedb.sh $(SOLC_BINARY)
	cd signer/fourbyte && go generate

# Genreates go source files containing the contract bytecode and abi.
embed-autonity-contract: $(GENERATED_BYTECODE) $(GENERATED_RAW_ABI) $(GENERATED_ABI)
$(GENERATED_BYTECODE) $(GENERATED_RAW_ABI) $(GENERATED_ABI): $(AUTONITY_CONTRACT_DIR)/*.sol $(SOLC_BINARY) $(GOBINDATA_BINARY)
	$(call gen-contract,$(GENERATED_CONTRACT_DIR),$(AUTONITY))
	$(SOLC_BINARY) --overwrite --abi --bin -o $(GENERATED_CONTRACT_DIR) $(AUTONITY_CONTRACT_DIR)/Upgrade_test.sol
	@echo Generating $(GENERATED_UPGRADE_ABI)
	@echo 'package generated' > $(GENERATED_UPGRADE_ABI)
	@echo -n 'const UpgradeTestAbi = `' >> $(GENERATED_UPGRADE_ABI)
	@cat  $(GENERATED_CONTRACT_DIR)/AutonityUpgradeTest.abi | json_pp  >> $(GENERATED_UPGRADE_ABI)
	@echo '`' >> $(GENERATED_UPGRADE_ABI)
	@gofmt -s -w $(GENERATED_UPGRADE_ABI)

	@echo Generating $(GENERATED_UPGRADE_BYTECODE)
	@echo 'package generated' > $(GENERATED_UPGRADE_BYTECODE)
	@echo -n 'const UpgradeTestBytecode = "' >> $(GENERATED_UPGRADE_BYTECODE)
	@cat  $(GENERATED_CONTRACT_DIR)/AutonityUpgradeTest.bin >> $(GENERATED_UPGRADE_BYTECODE)
	@echo '"' >> $(GENERATED_UPGRADE_BYTECODE)
	@gofmt -s -w $(GENERATED_UPGRADE_BYTECODE)

	# update 4byte selector for clef
	build/generate_4bytedb.sh $(SOLC_BINARY)
	cd signer/fourbyte && go generate

$(SOLC_BINARY):
	mkdir -p $(BINDIR)
	wget -O $(SOLC_BINARY) https://github.com/ethereum/solidity/releases/download/v$(SOLC_VERSION)/solc-static-linux
	chmod +x $(SOLC_BINARY)

$(GOBINDATA_BINARY):
	mkdir -p $(BINDIR)
	wget -O $(GOBINDATA_BINARY) https://github.com/kevinburke/go-bindata/releases/download/v$(GOBINDATA_VERSION)/go-bindata-linux-amd64
	chmod +x $(GOBINDATA_BINARY)

all: embed-autonity-contract embed-oracle-contract
	go run build/ci.go install
	make generate-go-bindings

android:
	go run build/ci.go aar --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/geth.aar\" to use the library."
	@echo "Import \"$(GOBIN)/geth-sources.jar\" to add javadocs"
	@echo "For more info see https://stackoverflow.com/questions/20994336/android-studio-how-to-attach-javadoc"

ios:
	go run build/ci.go xcode --local
	@echo "Done building."
	@echo "Import \"$(BINDIR)/autonity.framework\" to use the library."

test: all
	go run build/ci.go test -coverage

test-fast:
	go run build/ci.go test

test-race-all: all
	go run build/ci.go test -race
	make test-race

test-race:
	go test -race -v ./consensus/tendermint/... -parallel 1
	go test -race -v ./consensus/test/... -timeout 30m

# This runs the contract tests using truffle against an Autonity node instance.
test-contracts: autonity
	@# npm list returns 0 only if the package is not installed and the shell only
	@# executes the second part of an or statement if the first fails.
	@# Nov, 2022, the latest release of Truffle, v5.6.6 does not works for the tests.
	@echo "check and install truffle.js"
	@npm list truffle > /dev/null || npm install truffle
	@echo "check and install web3.js"
	@npm list web3 > /dev/null || npm install web3
	@echo "check and install truffle-assertions.js"
	@npm list truffle-assertions > /dev/null || npm install truffle-assertions
	@$(NPMBIN)/truffle version
	@cd $(AUTONITY_CONTRACT_TEST_DIR)/autonity/ && rm -Rdf ./data && ./autonity-start.sh &
	@# Autonity can take some time to start up so we ping its port till we see it is listening.
	@# The -z option to netcat exits with 0 only if the port at the given addresss is listening.
	@for x in {1..10}; do \
		nc -z localhost 8545 ; \
	    if [ $$? -eq 0 ] ; then \
	        break ; \
	    fi ; \
		echo waiting 2 more seconds for autonity to start ; \
	    sleep 2 ; \
	done
	@cd $(AUTONITY_CONTRACT_TEST_DIR) && $(NPMBIN)/truffle test test.js --network autonity && cd -
	@cd $(AUTONITY_CONTRACT_TEST_DIR) && $(NPMBIN)/truffle test oracle.js && cd -

docker-e2e-test: embed-autonity-contract
	build/env.sh go run build/ci.go install
	cd docker_e2e_test && sudo python3 test_via_docker.py ..

mock-gen:
	mockgen -source=consensus/tendermint/core/core_backend.go -package=core -destination=consensus/tendermint/core/backend_mock.go
	mockgen -source=consensus/protocol.go -package=consensus -destination=consensus/protocol_mock.go
	mockgen -source=consensus/consensus.go -package=consensus -destination=consensus/consensus_mock.go

lint-dead:
	@./.github/tools/golangci-lint run \
		--config ./.golangci/step_dead.yml

lint: embed-autonity-contract
	@echo "--> Running linter for code diff versus commit $(LATEST_COMMIT)"
	@./.github/tools/golangci-lint run \
	    --new-from-rev=$(LATEST_COMMIT) \
	    --config ./.golangci/step1.yml \
	    --exclude "which can be annoying to use"

	@./.github/tools/golangci-lint run \
	    --new-from-rev=$(LATEST_COMMIT) \
	    --config ./.golangci/step2.yml

	@./.github/tools/golangci-lint run \
	    --new-from-rev=$(LATEST_COMMIT) \
	    --config ./.golangci/step3.yml

	@./.github/tools/golangci-lint run \
	    --new-from-rev=$(LATEST_COMMIT) \
	    --config ./.golangci/step4.yml

lint-ci: lint

test-deps:
	go get golang.org/x/tools/cmd/cover
	go get github.com/mattn/goveralls
	cd tests/testdata && git checkout b5eb9900ee2147b40d3e681fe86efa4fd693959a

lint-deps:
	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b ./build/bin v1.46.2

clean:
	go clean -cache
	rm -fr build/_workspace/pkg/ $(BINDIR)/*
	rm -rf $(GENERATED_CONTRACT_DIR)

# The devtools target installs tools required for 'go generate'.
# You need to put $BINDIR (or $GOPATH/bin) in your PATH to use 'go generate'.

devtools:
	go get -u github.com/golang/mock/mockgen
	env BINDIR= go get -u golang.org/x/tools/cmd/stringer
	env BINDIR= go get -u github.com/kevinburke/go-bindata/go-bindata
	env BINDIR= go get -u github.com/fjl/gencodec
	env BINDIR= go get -u github.com/golang/protobuf/protoc-gen-go
	env BINDIR= go install ./cmd/abigen
	@type "npm" 2> /dev/null || echo 'Please install node.js and npm'
	@type "solc" 2> /dev/null || echo 'Please install solc'
	@type "protoc" 2> /dev/null || echo 'Please install protoc'
