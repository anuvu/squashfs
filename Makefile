VERSION := $(shell x=$$(git describe --tags --long 2>/dev/null) && echo $${x\#v} || echo unknown)
VERSION_SUFFIX := $(shell [ -z "$$(git status --porcelain --untracked-files=no 2>/dev/null)" ] || echo -dirty)
VERSION_FULL := $(VERSION)$(VERSION_SUFFIX)
X_VERSION := -X main.version=$(VERSION_FULL)
EXTLD_STATIC := -extldflags "-lzstd -lz -llzma -llz4 -static"
ROOTCMD ?= $(shell [ `id -u` = 0 ] && exit 0; command -v fakeroot 2>/dev/null || echo sudo)
GO_LIB_FILES := $(wildcard *.go)
GO_TOOL_FILES := $(wildcard squashtool/*.go)
SQUASHTOOL := squashtool/squashtool
SQUASHFS_IMAGES := noroot.squashfs root.squashfs

all: .build $(SQUASHTOOL)

static: $(SQUASHTOOL).static

.build: $(GO_LIB_FILES)
	go build ./...
	@touch "$@"

$(SQUASHTOOL): $(GO_LIB_FILES) $(GO_TOOL_FILES)
	cd $(dir $@) && go build -o $(notdir $@) -ldflags '$(X_VERSION)' ./...

$(SQUASHTOOL).static: $(GO_LIB_FILES) $(GO_TOOL_FILES)
	cd $(dir $@) && go build -o $(notdir $@) $(BUILD_FLAGS) -ldflags '$(X_VERSION) $(EXTLD_STATIC)' ./...

test: $(SQUASHTOOL) images
	./$(SQUASHTOOL) test-main noroot.squashfs
	./$(SQUASHTOOL) list noroot.squashfs

images: $(SQUASHFS_IMAGES)

noroot.squashfs: make-test-squashfs
	./make-test-squashfs $@

root.squashfs: make-test-squashfs
	$(ROOTCMD) ./make-test-squashfs $@

clean:
	rm -f $(SQUASHTOOL) $(SQUASHTOOL).static $(SQUASHFS_IMAGES) .build

.PHONY: static all images
