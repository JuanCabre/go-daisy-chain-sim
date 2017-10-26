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
	// Seed random number generator to produce different results every time
	rand.Seed(time.Now().UTC().UnixNano())

	params := Produce()

	w0 := Work(params)
	w1 := Work(params)
	w2 := Work(params)
	w3 := Work(params)

	r := merge(w0, w1, w2, w3)

	Consume(r)
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

	out := make(chan Parameters)
	runs := uint32(1000)

	allSymbols := []uint32{8, 32, 64, 128}
	allSymbolSizes := []uint32{8}
	allFieldSizes := []string{"Binary8"}
	allRelaysCounts := []int{1, 3, 5, 7}
	allRecLocs := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	allEpsilons := []float64{0.1, 0.2, 0.3, 0.4, 0.5}

	var totalRuns uint64
	testCount := uint64(0)
	totalRuns = uint64(len(allSymbols)) *
		uint64(len(allSymbolSizes)) *
		uint64(len(allFieldSizes)) *
		uint64(len(allRelaysCounts)) *
		uint64(len(allRecLocs)) *
		uint64(len(allEpsilons))
	go func() {
		for _, symbols := range allSymbols {
			for _, symbolSize := range allSymbolSizes {
				for _, fieldSize := range allFieldSizes {
					for _, relaysCount := range allRelaysCounts {
						for _, recLoc := range allRecLocs {
							for _, epsilon := range allEpsilons {
								testCount += 1
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
func Work(in chan Parameters) chan Result {

	out := make(chan Result)

	go func() {
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

			// These lines show the API to clean the memory used by the factories
			defer kodo.DeleteEncoderFactory(EncoderFactory)
			defer kodo.DeleteDecoderFactory(DecoderFactory)

			RunTests := func(runs uint32) {
				// Build the encoder
				encoder := EncoderFactory.Build()
				defer kodo.DeleteEncoder(encoder)

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

					// Set the storage for the decoder
					dataOut := make([]uint8, len(dataIn))
					decoder.SetMutableSymbols(&dataOut[0], decoder.BlockSize())
					dataRec := make([]uint8, len(dataIn))
					recoder.SetMutableSymbols(&dataRec[0], recoder.BlockSize())

					Tx := uint64(0)

					for !decoder.IsComplete() {
						// Encode the packet into the payload buffer...
						encoder.WritePayload(&payload[0])
						Tx++
						// ...chek if we drop the packet of the encoder...
						prevTx := rand.Float64() > epsilon
						// ...pass that packet to the relays...
						for r := 0; r < relCount; r++ {
							// ...if the relay is the recoder
							if r == recPos {
								// ...feed the payload if the prevTx succeeded...
								if prevTx {
									recoder.ReadPayload(&payload[0])
								}
								// ...and write the recoded payload no matter what the prevTx was
								recoder.WritePayload(&payload[0])
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
							decoder.ReadPayload(&payload[0])
						}
					}

					// Check if we properly decoded the data
					for i, v := range dataIn {
						if v != dataOut[i] {
							fmt.Println("Unexpected failure to decode")
							fmt.Println("Please file a bug report :)")
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
			}
			RunTests(runs)
		}
		close(out)
	}()
	return out
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
			log.Fatal(err)
		}
	}
	w.Flush()
}

func merge(cs ...<-chan Result) <-chan Result {
	var wg sync.WaitGroup
	out := make(chan Result)

	// Start an output goroutine for each input channel in cs.  output
	// copies values from c to out until c is closed, then calls wg.Done.
	output := func(c <-chan Result) {
		for n := range c {
			out <- n
		}
		wg.Done()
	}
	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}

	// Start a goroutine to close out once all the output goroutines are
	// done.  This must start after the wg.Add call.
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}
