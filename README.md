# golang library for reading squashfs filesystems.
This is a golang library that provides access to squashfs filesystems.

It uses the the libsquashfs C library from [squashfs-tools-ng](https://github.com/AgentD/squashfs-tools-ng).

It will build against 1.0.0, but the Read operations require a fix for [squashfs-tools-ng/#58](https://github.com/AgentD/squashfs-tools-ng/issues/58).

There is really good doc of squashfs format at [doc/format.adoc](https://github.com/AgentD/squashfs-tools-ng/blob/master/doc/format.adoc)

## Build setup
Most of this is handled by `setup` program.  But if you want to do it on your own, the following is a summary of what `setup` will do.

I'm assuming you have 'git'.  Get that with apt or yum.

 * get squashfs-tools-ng

        $ git clone https://github.com/AgentD/squashfs-tools-ng.git

 * get go-squashfs

        $ git clone https://github.com/anuvu/squashfs go-squashfs


 * Get build deps

   * Ubuntu

          $ sudo apt-get install --no-install-recommends  --assume-yes \
              autoconf automake make autogen autoconf libtool binutils \
              git squashfs-tools

          $ sudo apt-get install --no-install-recommends --assume-yes \
             libzstd-dev zlib1g-dev liblz4-dev libc6-dev liblzma-dev

   * Centos 7

          # we need epel for libzstd
          $ sudo yum install --assumeyes https://dl.fedoraproject.org/pub/epel/epel-release-latest-7.noarch.rpm

          $ sudo yum install --assumeyes \
              git make autogen automake autoconf \
              libtool binutils squashfs-tools
    
          $ sudo yum install --assumeyes \
             libzstd-devel libzstd-static \
             zlib-devel zlib-static \
             lz4-devel lz4-static \
             glibc-devel glibc-static \
             xz xz-devel


 * Get golang.  You can/should figure this out yourself, but here is one way:

        $ ver=1.14.4
        $ majmin=${ver%.*}
        $ curl https://dl.google.com/go/go${ver}.linux-amd64.tar.gz > go.tar.gz
 
        # this creates /usr/lib/go-1.14
        $ sudo tar -C /usr/lib -xvf go.tar.gz --show-transformed-names --transform "s/^go/go-$majmin/"
        $ sudo ln -sf ../lib/go-$majmin/bin/go /usr/bin/go


 * install into mylocal="$HOME/lib"

        $ mylocal="$HOME/mylocal"
        $ export LD_LIBRARY_PATH=$mylocal/lib${LD_LIBRARY_PATH:+:${LD_LIBRARY_PATH}}
        $ export PKG_CONFIG_PATH=$mylocal/lib/pkgconfig${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}

 * build and install squashfs-tools-ng

        $ cd squashfs-tools-ng
        $ ./autogen.sh
        $ ./configure --prefix=$mylocal
        $ make
        $ make install

 * build go-squashfs

        $ cd go-squashfs
        $ make
