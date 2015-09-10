#!/bin/bash -e

go fmt
go test
go test -race

ROOT=profiling
rm -rf ${ROOT}
mkdir -p ${ROOT}

# CPU profiling
go test -bench . -cpuprofile=${ROOT}/cpuprof.out
go tool pprof -pdf ./statesman.test ${ROOT}/cpuprof.out > ${ROOT}/cpuprof.pdf
mv statesman.test ${ROOT}/cpuprof.test

# MEM profiling
go test -bench . -memprofile=${ROOT}/memprof.out -memprofilerate=1
go tool pprof --alloc_space -pdf ./statesman.test ${ROOT}/memprof.out > ${ROOT}/memprof.pdf
mv statesman.test ${ROOT}/memprof.test

# Code coverage
go test -coverprofile=${ROOT}/coverage.out
go tool cover -html=${ROOT}/coverage.out -o ${ROOT}/coverage.html
