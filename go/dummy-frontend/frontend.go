// SPDX-License-Identifier: MIT
// Copyright (c) 2022 University of California, Riverside

package main

// #cgo pkg-config: libdpdk
// #cgo CFLAGS: -I${SRCDIR}/../../src/include
// #cgo LDFLAGS: -L${SRCDIR}/../../src
// #cgo rte_ring LDFLAGS: -l:io_rte_ring.o
// #cgo sk_msg LDFLAGS: -l:io_sk_msg.o -lbpf
//
// #include <errno.h>
// #include <stdint.h>
// #include <stdio.h>
// #include <stdlib.h>
// #include <string.h>
//
// #include <rte_branch_prediction.h>
// #include <rte_eal.h>
// #include <rte_errno.h>
// #include <rte_memzone.h>
//
// #include "http.h"
// #include "io.h"
// #include "spright.h"
//
// static void *argv_create(int argc)
// {
// 	char *argv = NULL;
//
// 	argv = malloc(argc * sizeof(char *));
// 	if (unlikely(argv == NULL)) {
// 		fprintf(stderr, "malloc() error: %s\n", strerror(errno));
// 		return NULL;
// 	}
//
//	return argv;
// }
//
// static void argv_destroy(void *argv)
// {
// 	free(argv);
// }
//
// static int nf_init(int argc, char **argv)
// {
// 	const struct rte_memzone *memzone = NULL;
// 	int ret;
//
// 	ret = rte_eal_init(argc, argv);
// 	if (unlikely(ret == -1)) {
// 		fprintf(stderr, "rte_eal_init() error: %s\n",
// 		        rte_strerror(rte_errno));
// 		goto error_0;
// 	}
//
// 	argc -= ret;
// 	argv += ret;
//
// 	if (unlikely(argc == 1)) {
// 		fprintf(stderr, "Network Function ID not provided\n");
// 		goto error_1;
// 	}
//
// 	errno = 0;
// 	node_id = strtol(argv[1], NULL, 10);
// 	if (unlikely(errno != 0 || node_id < 1)) {
// 		fprintf(stderr, "Invalid value for Network Function ID\n");
// 		goto error_1;
// 	}
//
// 	memzone = rte_memzone_lookup(MEMZONE_NAME);
// 	if (unlikely(memzone == NULL)) {
// 		fprintf(stderr, "rte_memzone_lookup() error\n");
// 		goto error_1;
// 	}
//
// 	cfg = memzone->addr;
//
// 	ret = io_init();
// 	if (unlikely(ret == -1)) {
// 		fprintf(stderr, "io_init() error\n");
// 		goto error_1;
// 	}
//
// 	return 0;
//
// error_1:
// 	rte_eal_cleanup();
// error_0:
// 	return -1;
// }
//
// static int nf_exit(void)
// {
// 	int ret;
//
// 	ret = io_exit();
// 	if (unlikely(ret == -1)) {
// 		fprintf(stderr, "io_exit() error\n");
// 		return -1;
// 	}
//
// 	ret = rte_eal_cleanup();
// 	if (unlikely(ret < 0)) {
// 		fprintf(stderr, "rte_eal_cleanup() error: %s\n",
// 		        rte_strerror(-ret));
// 		return -1;
// 	}
//
// 	return 0;
// }
//
// static int nf_io_rx(struct http_transaction **txn)
// {
// 	return io_rx((void **)txn);
// }
//
// static int nf_io_tx(struct http_transaction *txn, uint8_t next_nf)
// {
// 	return io_tx(txn, next_nf);
// }
//
// static struct http_transaction *txn_create(void)
// {
// 	struct http_transaction *txn;
// 	int ret;
//
// 	ret = rte_mempool_get(cfg->mempool, (void **)&txn);
// 	if (unlikely(ret < 0)) {
// 		fprintf(stderr, "rte_mempool_get() error: %s\n",
// 		        rte_strerror(-ret));
// 		return NULL;
// 	}
//
// 	return txn;
// }
//
// static void txn_delete(struct http_transaction *txn)
// {
// 	rte_mempool_put(cfg->mempool, txn);
// }
//
// static uint8_t route(struct http_transaction *txn)
// {
// 	uint8_t next_nf;
//
// 	txn->hop_count++;
//
// 	if (likely(txn->hop_count < cfg->route[txn->route_id].length)) {
// 		next_nf = cfg->route[txn->route_id].node[txn->hop_count];
// 	} else {
// 		next_nf = 0;
// 	}
// 	return next_nf;
// }
//
// static int get_num_workers(uint8_t nf_id)
// {
// 	uint8_t num_workers = cfg->nf[nf_id - 1].n_threads;
// 	return (int) num_workers;
// }
// static uint8_t get_route_len(uint8_t route_id)
// {
// 	return cfg->route[route_id].length;
// }
// static uint8_t get_route_hop(uint8_t route_id, uint8_t hop_idx)
// {
// 	return cfg->route[route_id].node[hop_idx];
// }
// static char* get_nf_name(uint8_t nf_id)
// {
// 	return cfg->nf[nf_id - 1].name;;
// }
// static uint8_t get_n_nfs()
// {
// 	return cfg->n_nfs;
// }
import "C"

import (
	"errors"
	"os"
	"unsafe"
	"fmt"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	// pb "frontend/hipstershop"
)

var (
	nfID uint8
	numWorkers int
	nfName string = "FrontendService"
	nfNameToIdMap map[string]uint8
	FrontendService = &server{}
)

var log *logrus.Logger

type ReceiveChannel struct {
    Transaction *C.struct_http_transaction
}

type TransmitChannel struct {
    Transaction *C.struct_http_transaction
	NextNF C.uint8_t
}

func init() {
	log = logrus.New()
	log.Level = logrus.DebugLevel
	log.Formatter = &logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "severity",
			logrus.FieldKeyMsg:   "message",
		},
		TimestampFormat: time.RFC3339Nano,
	}
	log.Out = os.Stdout
}

func nfInit() error {
	argc := C.int(len(os.Args))
	argv := (*[0xff]*C.char)(C.argv_create(argc))
	defer C.argv_destroy(unsafe.Pointer(argv))

	for i := 0; i < int(argc); i++ {
		argv[i] = C.CString(os.Args[i])
		defer C.free(unsafe.Pointer(argv[i]))
	}

	ret := C.nf_init(argc, (**C.char)(unsafe.Pointer(argv)))
	if (ret == -1) {
		return errors.New("nf_init() error")
	}

	nfID_int, _ := strconv.Atoi(os.Args[8])
	nfID = uint8(nfID_int)
	
	numWorkers = int(C.get_num_workers(C.uchar(nfID)))
	
	// Initialize the NF Name to NF ID map
	numNF := C.get_n_nfs()
	nfNameToIdMap = make(map[string]uint8)
	for idx := 1; idx <= int(numNF); idx++ {
		nfNameToIdMap[C.GoString(C.get_nf_name(C.uchar(uint8(idx))))] = uint8(idx)
	}
	fmt.Printf("nfNameToIdMap: %v\n", nfNameToIdMap)

	if nfName != C.GoString(C.get_nf_name(C.uchar(nfID))) {
		log.Error("!!Function name does not match with the config!!")
	}

	return nil
}

func nfExit() error {
	ret := C.nf_exit()
	if (ret == -1) {
		return errors.New("nf_exit() error")
	}

	return nil
}

func ioRx(rxChan chan<- ReceiveChannel) {
	log.Infof("Receiver Thread started")
	for {
		var txn = (*C.struct_http_transaction)(C.NULL)

		ret := C.nf_io_rx(&txn)
		if (ret == -1) {
			panic(errors.New("nf_io_rx() error"))
		}
	
		rxChan <- ReceiveChannel{Transaction: txn}
	}
}

func ioTx(txChan <-chan TransmitChannel) {
	log.Infof("Transmiter Thread started")
	for t := range txChan {
		ret := C.nf_io_tx(t.Transaction, t.NextNF)
		if (ret == -1) {
			panic(errors.New("nf_io_tx() error"))
		}
	}
}

func txnCreate() *C.struct_http_transaction {
	return C.txn_create()
}

func txnDelete(txn *C.struct_http_transaction) {
	C.txn_delete(txn)
}

func nfWorker(threadID int, rxChan <-chan ReceiveChannel, txChan chan<- TransmitChannel) {
	fmt.Printf("Worker Thread %v started\n", threadID)

	for rx := range rxChan {
		// fmt.Printf("Thread %v: Received msg\n", threadID)
		time.Sleep(1 * time.Second)

		txn := rx.Transaction
		var next_nf C.uint8_t
		txn.hop_count = txn.hop_count + C.uchar(1)

		next_nf = nfDispatcher(txn) // run dispatcher to select the handler

		// fmt.Printf("Next NF: %v, Current Hop: %v\n", next_nf, txn.hop_count)
		txChan <- TransmitChannel{Transaction: txn, NextNF: next_nf}
	}
}

// txHandler sets up the current NF as the caller and
// writes the name of remote handler to called in the next function
func txHandler(next_rpcHandler string, txn *C.struct_http_transaction) {
	callerNF := C.CString(nfName) // There is one copy
	defer C.free(unsafe.Pointer(callerNF))
	C.strcpy(&txn.caller_nf[0], callerNF) // There is another one copy

	cs := C.CString(next_rpcHandler) // There is one copy
	defer C.free(unsafe.Pointer(cs))
	C.strcpy(&txn.rpc_handler[0], cs) // There is another one copy
}

func nfDispatcher(txn *C.struct_http_transaction) C.uint8_t {
	var next_nf C.uint8_t
	var next_rpcHandler string

	rpcHandler := C.GoString(&txn.rpc_handler[0])
	// fmt.Printf("Handler %v() in %v gets called\n", rpcHandler, nfName)

	if rpcHandler == "" {
		fmt.Printf("OnlineBoutiqueEntryHandler() in %v gets called\n", nfName)
		next_nf, next_rpcHandler = OnlineBoutiqueEntryHandler(txn)
	} else if rpcHandler == "ListRecommendationsResponseHandler" {
		next_nf, next_rpcHandler = ListRecommendationsResponseHandler(txn)
	} else if rpcHandler == "GetAdsResponseHandler" {
		next_nf, next_rpcHandler = GetAdsResponseHandler(txn)
	} else if rpcHandler == "GetQuoteResponseHandler" {
		next_nf, next_rpcHandler = GetQuoteResponseHandler(txn)
	} else {
		log.Errorf("%v is not supported by %v!", rpcHandler, nfName)
	}

	txHandler(next_rpcHandler, txn)
	fmt.Printf("%v will call %v() in %v\n", nfName, next_rpcHandler, next_nf)

	return next_nf
}

func OnlineBoutiqueEntryHandler(txn *C.struct_http_transaction) (C.uint8_t, string) {
	var next_nf C.uint8_t
	var next_rpcHandler string
	var nextNFName string

	if txn.route_id == C.uchar(1) {
		nextNFName = "RecommendationService"
		next_rpcHandler = "ListRecommendationsHandler"
	} else if txn.route_id == C.uchar(2) {
		nextNFName = "AdService"
		next_rpcHandler = "GetAdsHandler"
	} else if txn.route_id == C.uchar(3) {
		nextNFName = "ShippingService"
		next_rpcHandler = "GetQuoteHandler"
	} else {
		nextNFName = ""
		next_rpcHandler = ""
	}

	if val, ok := nfNameToIdMap[nextNFName]; ok {
		next_nf = C.uchar(val)
	} else {
		// TODO: add error codes support in the txn structure
		log.Error("Unknown service! Report 500 or 501 error to Gateway")
		next_nf = 0
		next_rpcHandler = "ErrorResponseHandler"
		txn.hop_count = C.uchar(11) // HACK - force gateway to return a response
	}

	return next_nf, next_rpcHandler
}

// Return the recommendations to the user
func ListRecommendationsResponseHandler(txn *C.struct_http_transaction) (C.uint8_t, string) {
	var next_nf C.uint8_t
	var next_rpcHandler string

	log.Info("Frontend returns the recommendations to the user")
	next_nf = 0
	next_rpcHandler = ""

	txn.hop_count = C.uchar(11) // HACK - force gateway to return a response

	return next_nf, next_rpcHandler
}

func GetAdsResponseHandler(txn *C.struct_http_transaction) (C.uint8_t, string) {
	var next_nf C.uint8_t
	var next_rpcHandler string

	log.Info("Frontend returns the ads to the user")
	next_nf = 0
	next_rpcHandler = ""

	txn.hop_count = C.uchar(11) // HACK - force gateway to return a response

	return next_nf, next_rpcHandler
}

func GetQuoteResponseHandler(txn *C.struct_http_transaction) (C.uint8_t, string) {
	var next_nf C.uint8_t
	var next_rpcHandler string

	log.Info("Frontend returns the shipping cost to the user")
	next_nf = 0
	next_rpcHandler = ""

	txn.hop_count = C.uchar(11) // HACK - force gateway to return a response

	return next_nf, next_rpcHandler
}

func nf() error {
	RxChan := make(chan ReceiveChannel)
	TxChan := make(chan TransmitChannel)

	log.Infof("%v (ID: %v) is creating %v worker threads...", nfName, nfID, numWorkers)
	for idx := 1; idx <= numWorkers; idx++ {
		go nfWorker(idx, RxChan, TxChan)
	}
	
	go ioRx(RxChan)
	
	ioTx(TxChan)

	close(RxChan)
	close(TxChan)

	return nil
}

func main() {
	var err error

	err = nfInit()
	if err != nil {
		panic(err)
	}

	err = nf()
	if err != nil {
		panic(err)
	}

	err = nfExit()
	if err != nil {
		panic(err)
	}
}

// server controls Handlers.
type server struct{}

func (s *server) FrontendPrint() {
	fmt.Printf("This is Frontend Service\n")
}
