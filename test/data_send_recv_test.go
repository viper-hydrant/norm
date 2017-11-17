package norm_test

import (
	"testing"
	"fmt"
	"log"
	"github.com/viper-hydrant/norm"
	"sync"
	"context"
	"time"
	"os"
	"bytes"
	"sync/atomic"
	"github.com/stretchr/testify/assert"
	"strings"
)

const (
	testDataSendRecvObjectSize  = 5000000
	testDataSendRecvObjectCount = 100
)

var (
	commPort = 10024
)

func TestDataSendRecv(t *testing.T) {

	a := assert.New(t)

	wg := new(sync.WaitGroup)
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer func(){
		fmt.Fprint(os.Stderr, "===> TestDataSendRecv: Calling c.wg.Done()\n")
		wg.Done()
		fmt.Fprint(os.Stderr, "===> TestDataSendRecv: After c.wg.Done()\n")
	}()
	defer cancel()
	sChan := make(chan []byte)

	var objectsReceived int32 = 0
	expectedObject := bytes.Repeat([]byte{'a'}, int(testDataSendRecvObjectSize))

	wgTest := &sync.WaitGroup{}
	wgTest.Add(1)
	defer wgTest.Wait()

	InitRecv(wg,
		cancel,
		ctx,
		"",
		3,
		false,
		"127.0.0.1",
		commPort,
		false,
		7,
		func(actual []byte) {
			log.Println("Test: data object received callback")
			a.Equal(expectedObject, actual, "Expected and actual objects aren't equal!")

			totalProcessed := atomic.AddInt32(&objectsReceived, 1)
			log.Printf("Test: total objects processed: %v", totalProcessed)
			if testDataSendRecvObjectCount == totalProcessed {
				wgTest.Done()
				log.Println("Test: called test workgroup done")
			}
		},
		&sync.Mutex{})
	InitSend(sChan,
		wg,
		cancel,
		ctx,
		"",
		3,
		false,
		"127.0.0.1",
		commPort,
		false,
		testDataSendRecvObjectCount,
		0,
		0,
		0,
		testDataSendRecvObjectSize,
		6,
		7,
		&sync.Mutex{})

	for i := 0; testDataSendRecvObjectCount > i; i++ {
		sChan <- bytes.Repeat([]byte{'a'}, int(testDataSendRecvObjectSize))
		time.Sleep(50 * time.Millisecond)
	}
}

func TestDataSendRecvX100(t *testing.T) {
	for i := 0; 100 > i; i ++ {
		output := strings.Repeat("*", 30) + " Running iteration %d " + strings.Repeat("*", 30) + "\n"
		log.Printf(output, i)
		TestDataSendRecv(t)
		time.Sleep(3 * time.Second)
		commPort++
	}
}

func InitSend(sChan chan []byte,
	wg *sync.WaitGroup,
	cancel context.CancelFunc,
	ctx context.Context,
	debugLog string,
	normDebugLevel uint,
	debugInstance bool,
	sAddr string,
	sPort int,
	trace bool,
	maxCt uint,
	sTxRate float64,
	sTxRateMin float64,
	sTxRateMax float64,
	size int,
	sNodeId uint,
	rNodeId uint,
	nilOrLock sync.Locker) {

	go func() {
		wg.Add(1)
		defer func() {
			fmt.Fprint(os.Stderr, "===> InitSend: Calling c.wg.Done()\n")
			wg.Done()
			fmt.Fprint(os.Stderr, "===> InitSend: After c.wg.Done()\n")
		}()
		defer cancel()
		i, err := norm.Create_instance(wg, nilOrLock)
		if err != nil {
			log.Println(err)
			return
		}
		defer i.Destroy()
		if 0 < len(debugLog) {
			log.Println("debug log:", debugLog)
			i.Open_debug_log(debugLog)
			defer i.Close_debug_log()
		}
		log.Println("norm version:", i.Get_version())
		old := i.Get_debug_level()
		i.Set_debug_level(normDebugLevel)
		log.Printf("norm debug level: %v -> %v\n", old, normDebugLevel)
		i.Set_debug(debugInstance)
		log.Printf("sender session: %v:%v\n", sAddr, sPort)
		sess, err := i.Create_session(sAddr, sPort, norm.Node_id(sNodeId))
		if err != nil {
			log.Println(err)
			return
		}
		log.Println("id:", sess.Get_local_node_id())
		log.Println("trace:", trace)
		sess.Set_message_trace(trace)
		sess.Set_backoff_factor(2.0)
		sess.Set_group_size(10)
		sess.Set_tx_only(true, false)
		sess.Set_events(norm.Event_type_all &^ (norm.Event_type_grtt_updated | norm.Event_type_send_error | norm.Event_type_tx_rate_changed))
		defer sess.Destroy()
		if !sess.Add_acking_node(norm.Node_id(rNodeId)) {
			log.Println("Add_acking_node", err)
			return
		}
		sess.Set_tx_cache_bounds(1e+6*100, 1000, 2000)
		switch {
		case 0 < sTxRate:
			log.Printf("set_tx_rate: %.f\n", sTxRate)
			sess.Set_tx_rate(sTxRate)
		case 0 < sTxRateMin || 0 < sTxRateMax:
			log.Println("setting cc: true, true")
			sess.Set_congestion_control(true, true)
			log.Printf("Set_tx_rate_bounds: rate_min: %.f, rate_max: %.f\n", sTxRateMin, sTxRateMax)
			sess.Set_tx_rate_bounds(sTxRateMin, sTxRateMax)
		default:
			log.Println("setting cc: true, true")
			sess.Set_congestion_control(true, true)
		}
		if !sess.Start_sender(0, 1024*1024*4, 0, 0, 0, 0) {
			log.Println(err)
			return
		}
		defer sess.Stop_sender()
		ct := uint(1)

		cmd := []byte("Command is Hello!")
		if !sess.Send_command(cmd, false) {
			log.Println("tx command failed")
			return
		}
		log.Println("tx cmd:", string(cmd))

		var obj *norm.Object
		dataEnqueue := func(data []byte) {
			if ct <= maxCt {
				_, err := sess.Data_enqueue(
					data,
					[]byte(fmt.Sprintf("Data, count: %v of %v, size: %v", ct, maxCt, size)),
				)
				if err != nil {
					log.Println(err)
					return
				}
				log.Printf("data enqueue: %v of %v(root select)\n", ct, maxCt)
				ct++
			} else {
				if obj != nil {
					sess.Set_watermark(obj, true)
					obj = nil
				}
			}
		}

		for {
			select {
			case data := <-sChan:
				dataEnqueue(data)
			case e := <-sess.Events():
				switch e.Type {
				case norm.Event_type_send_error:
					log.Println(e)
				case norm.Event_type_tx_rate_changed:
					log.Println(e, e.Tx_rate)
				case norm.Event_type_grtt_updated, norm.Event_type_tx_object_sent,
					norm.Event_type_tx_object_purged:
					log.Println(e)
				case norm.Event_type_tx_flush_completed:
					log.Println(e)
					select {
					case data := <-sChan:
						dataEnqueue(data)
					default:
					}
				default:
					log.Println("main:", e)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func InitRecv(wg *sync.WaitGroup,
	cancel context.CancelFunc,
	ctx context.Context,
	debugLog string,
	normDebugLevel uint,
	debugInstance bool,
	rAddr string,
	rPort int,
	trace bool,
	rNodeId uint,
	rCallback func([]byte),
	nilOrLock sync.Locker) {
	go func() {
		fmt.Fprint(os.Stderr, "===> InitRecv: Calling c.wg.Add(1)\n")
		wg.Add(1)
		fmt.Fprint(os.Stderr, "===> InitRecv: After c.wg.Add(1)\n")
		defer func() {
			fmt.Fprint(os.Stderr, "===> InitRecv: Calling c.wg.Done()\n")
			wg.Done()
			fmt.Fprint(os.Stderr, "===> InitRecv: After c.wg.Done()\n")
		}()
		defer cancel()
		i, err := norm.Create_instance(wg, nilOrLock)
		if err != nil {
			log.Println(err)
			return
		}
		defer i.Destroy()
		if 0 < len(debugLog) {
			log.Println("debug log:", debugLog)
			i.Open_debug_log(debugLog)
			defer i.Close_debug_log()
		}
		old := i.Get_debug_level()
		i.Set_debug_level(normDebugLevel)
		log.Printf("norm debug level: %v -> %v\n", old, normDebugLevel)
		i.Set_debug(debugInstance)
		log.Printf("receiver session: %v:%v\n", rAddr, rPort)
		sess, err := i.Create_session(rAddr, rPort, norm.Node_id(rNodeId))
		if err != nil {
			log.Println(err)
		}
		defer sess.Destroy()
		log.Println("id:", sess.Get_local_node_id())
		sess.Set_message_trace(trace)
		sess.Set_events(norm.Event_type_all &^ (norm.Event_type_grtt_updated | norm.Event_type_rx_object_updated | norm.Event_type_rx_object_updated))
		sess.Set_default_unicast_nack(true)
		sess.Set_rx_cache_limit(4096)
		if !sess.Start_receiver(1024 * 1024) {
			log.Println(err)
			return
		}
		defer sess.Stop_receiver()
		if !sess.Set_rx_socket_buffer(4 * 1024 * 1024) {
			log.Println("Set_rx_socket_buffer failed")
			return
		}
		var startTime time.Time
		for {
			select {
			case e := <-sess.Events():
				switch e.Type {
				case norm.Event_type_remote_sender_new, norm.Event_type_remote_sender_active, norm.Event_type_remote_sender_inactive:
					log.Println("main:", e, e.Node.Get_id(), e.Node.Get_address_all())
				case norm.Event_type_rx_cmd_new:
					log.Println(e, "rx command:", e.Node.Get_command())
				case norm.Event_type_rx_object_new:
					startTime = time.Now()
				case norm.Event_type_rx_object_info:
					log.Printf("main: %v, info: %v ", e, e.O.Get_info())
				case norm.Event_type_rx_object_updated:
					ocompleted := e.O.Get_size() - e.O.Get_bytes_pending()
					percent := 100.00 * (float32(ocompleted) / float32(e.O.Get_size()))
					log.Printf("Consumer: Id: %v, completion status %v/%v %6.2f%%\n", e.O.Id, ocompleted, e.O.Get_size(), percent)
				case norm.Event_type_rx_object_completed:
					duration := time.Now().Sub(startTime).Seconds()
					data := e.O.Data_access_data(true)
					log.Printf("id: %v, transfer duration: %.6f sec at %.3f kbps\n",
						e.O.Id,
						time.Now().Sub(startTime).Seconds(),
						8.0/1000.0*float64(data.Len())/duration,
					)
					log.Println("main:", e, "data size:", data.Len())
					if nil != rCallback {
						go rCallback(data.Bytes())
					}
				default:
					log.Println("main:", e, e.O)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
