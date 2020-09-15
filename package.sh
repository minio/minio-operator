#!/bin/bash

binary=$(basename "$(dirname "${1}")")
minisign -s "/media/${USER}/minio/minisign.key" \
         -x "${binary}.minisig" \
         -Sm "${1}" < "/media/${USER}/minio/minisign-passphrase"

cp -f "${binary}.minisig" "${1}.minisig"
cp -f LICENSE "$(dirname "${1}")"
tar -C "$(dirname "${1}")" --numeric-owner -czf "${binary}.tar.gz" kubectl*

if [ -f minio-operator_linux_amd64.minisig ]; then
    mv minio-operator_linux_amd64.minisig minio-operator.minisig
fi
