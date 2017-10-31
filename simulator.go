package main

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"gitlab.com/steinwurf/kodo-go/src/kodo"
)

func main() {

	// f, err := os.Create("out.trace")
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// err = trace.Start(f)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer trace.Stop()

	// Seed random number generator to produce different results every time
	rand.Seed(time.Now().UTC().UnixNano())

	params := Produce()

	workers := 32
	results := make(chan Result, workers)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go Work(params, results, &wg)
	}

	go func(wg *sync.WaitGroup) {
		wg.Wait()
		close(results)
	}(&wg)

	Consume(results)
}

type Parameters struct {
	symbols     uint32
	symbolSize  uint32
	fieldSize   string
	runs        uint32
	relaysCount int
	recLoc      int
	epsilon     float64
}

type Result struct {
	Parameters
	run uint32
	tx  uint64
}

// Produce will provide test parameters to the simmulation
func Produce() chan Parameters {
	runs := uint32(5000)

	// allSymbols := []uint32{8, 32, 64, 128, 256, 512}
	// allSymbolSizes := []uint32{8}
	// allFieldSizes := []string{"Binary8"}
	// allRelaysCounts := []int{1, 3, 5, 7, 9, 11, 13, 15}
	// allRecLocs := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	// allEpsilons := []float64{0.1, 0.2, 0.3, 0.4, 0.5}

	allSymbols := []uint32{8, 32}
	allSymbolSizes := []uint32{8}
	allFieldSizes := []string{"Binary8"}
	allRelaysCounts := []int{1, 3}
	allRecLocs := []int{0, 1}
	allEpsilons := []float64{0.1}

	var totalRuns uint64
	testCount := uint64(0)
	totalRuns = uint64(len(allSymbols)) *
		uint64(len(allSymbolSizes)) *
		uint64(len(allFieldSizes)) *
		uint64(len(allRelaysCounts)) *
		uint64(len(allRecLocs)) *
		uint64(len(allEpsilons))

	out := make(chan Parameters, 30)

	go func() {
		for _, symbols := range allSymbols {
			for _, symbolSize := range allSymbolSizes {
				for _, fieldSize := range allFieldSizes {
					for _, relaysCount := range allRelaysCounts {
						for _, recLoc := range allRecLocs {
							for _, epsilon := range allEpsilons {
								testCount++
								progress := float64(testCount) / float64(totalRuns)
								fmt.Println("Progress: ", progress, "%")

								// Do not run a simmulation without recoder
								if recLoc >= relaysCount {
									continue
								}
								r := Parameters{symbols: symbols,
									symbolSize:  symbolSize,
									fieldSize:   fieldSize,
									runs:        runs,
									relaysCount: relaysCount,
									recLoc:      recLoc,
									epsilon:     epsilon}

								out <- r
							}
						}
					}
				}
			}
		}
		close(out)
	}()
	return out
}

// Work will listen for parameters in the channel, will run the simmulation with
// the specified parameters, and will produce a result to the Result channel
func Work(in chan Parameters, out chan Result, wg *sync.WaitGroup) {
	defer wg.Done()

	for p := range in {
		// Set the number of symbols (i.e. the generation size in RLNC
		// terminology) and the size of a symbol in bytes
		symbols := p.symbols
		SymbolSize := p.symbolSize
		runs := p.runs
		relCount := p.relaysCount
		epsilon := p.epsilon
		recPos := p.recLoc

		var r Result
		r.Parameters = p

		// Initilization of encoder and decoder factories
		EncoderFactory := kodo.NewEncoderFactory(kodo.FullVector,
			kodo.Binary8, symbols, SymbolSize)
		DecoderFactory := kodo.NewDecoderFactory(kodo.FullVector,
			kodo.Binary8, symbols, SymbolSize)

		// Build the encoder
		encoder := EncoderFactory.Build()
		// defer kodo.DeleteEncoder(encoder)

		// Set systematic off
		encoder.SetSystematicOff()

		// Allocate some data to encode. In this case we make a buffer
		// with the same size as the encoder's block size (the max.
		// amount a single encoder can encode)
		dataIn := make([]uint8, encoder.BlockSize())

		// Just for fun - fill the data with random data
		for i := range dataIn {
			dataIn[i] = uint8(rand.Uint32())
		}

		// Assign the data buffer to the encoder so that we may start
		// to produce encoded symbols from it
		encoder.SetConstSymbols(&dataIn[0], symbols*SymbolSize)

		for i := uint32(0); i < runs; i++ {
			err := false
			decoder := DecoderFactory.Build()
			recoder := DecoderFactory.Build()

			// Allocate some storage for a "payload" the payload is what we would
			// eventually send over a network
			payload := make([]uint8, encoder.PayloadSize())
			recPayload := make([]uint8, encoder.PayloadSize())

			// Set the storage for the decoder
			dataOut := make([]uint8, len(dataIn))
			decoder.SetMutableSymbols(&dataOut[0], decoder.BlockSize())
			dataRec := make([]uint8, len(dataIn))
			recoder.SetMutableSymbols(&dataRec[0], recoder.BlockSize())

			Tx := uint64(0)
			Rx := uint32(0)

			for Rx < symbols || !decoder.IsComplete() {
				// Encode the packet into the payload buffer...
				// encoder.WritePayload(&payload[0])
				Tx++
				// ...chek if we drop the packet of the encoder...
				prevTx := rand.Float64() > epsilon
				// ...pass that packet to the relays...
				for rel := 0; rel < relCount; rel++ {
					// ...if the relay is the recoder
					if rel == recPos {
						// ...feed the payload if the prevTx succeeded...
						if prevTx {
							encoder.WritePayload(&payload[0])
							recoder.ReadPayload(&payload[0])
						}
						// ...and write the recoded payload no matter what the prevTx was
						// Inject the packet to the network
						prevTx = rand.Float64() > epsilon
					} else {
						if prevTx { // If the previous transmission succeeded,
							// ...check if the next link will drop the packet
							prevTx = rand.Float64() > epsilon
						}
					}
				}
				if prevTx {
					// ...pass that packet to the decoder...
					recoder.WritePayload(&recPayload[0])
					decoder.ReadPayload(&recPayload[0])
					Rx++
				}
			}

			// Check if we properly decoded the data
			for j, v := range dataIn {
				if v != dataOut[j] {
					fmt.Println("Unexpected failure to decode")
					fmt.Println("Please file a bug report :)")
					fmt.Println("Data in: ", dataIn)
					fmt.Println("Data rec: ", dataRec)
					fmt.Println("Data out: ", dataOut)
					fmt.Printf("Test: \nsymbols: %d \nsymbol size: %d \nrun: %d \nrelcount: %d \nepsilon: %3f \nrecPos: %d\n", p.symbols, p.symbolSize, i, p.relaysCount, p.epsilon, p.recLoc)
					// panic("Error decoding")
					err = true
					break
				}
			}

			if !err {
				r.run = i
				r.tx = Tx
				out <- r
			}

			kodo.DeleteDecoder(decoder)
			kodo.DeleteDecoder(recoder)
		}
		kodo.DeleteEncoder(encoder)
		kodo.DeleteEncoderFactory(EncoderFactory)
		kodo.DeleteDecoderFactory(DecoderFactory)
	}
}

// Consume will store in a file the results arriving from the channel r
func Consume(in <-chan Result) {
	f, err := os.Create(time.Now().Format("2006-01-02_15:04") + "_simm.csv")
	if err != nil {
		log.Fatal(err)
	}
	w := bufio.NewWriter(f)
	header := "Symbols,SymbolSize,FieldSize,RelaysCount,RecLoc,Epsilon,Run,Tx\n"
	w.WriteString(header)
	for r := range in {
		line := fmt.Sprintf("%d,%d,%s,%d,%d,%f,%d,%d\n", r.symbols, r.symbolSize, r.fieldSize, r.relaysCount, r.recLoc, r.epsilon, r.run, r.tx)
		_, err := w.WriteString(line)
		if err != nil {
			fmt.Println("Line is: ", line)
			log.Fatal(err)
		}
	}
	w.Flush()
}
