package main

import (
	"fmt"
	"math/rand"
	"time"

	"gitlab.com/steinwurf/kodo-go/src/kodo"
)

func main() {
	// Seed random number generator to produce different results every time
	rand.Seed(time.Now().UTC().UnixNano())

	// Set the number of symbols (i.e. the generation size in RLNC
	// terminology) and the size of a symbol in bytes
	var symbols, SymbolSize uint32 = 10, 100
	runs := 10

	// Initilization of encoder and decoder
	EncoderFactory := kodo.NewEncoderFactory(kodo.FullVector,
		kodo.Binary8, symbols, SymbolSize)
	DecoderFactory := kodo.NewDecoderFactory(kodo.FullVector,
		kodo.Binary8, symbols, SymbolSize)

	// These lines show the API to clean the memory used by the factories
	defer kodo.DeleteEncoderFactory(EncoderFactory)
	defer kodo.DeleteDecoderFactory(DecoderFactory)

	// Build the encoder
	encoder := EncoderFactory.Build()
	defer kodo.DeleteEncoder(encoder)

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

	for i := 0; i < runs; i++ {
		decoder := DecoderFactory.Build()
		recoder := DecoderFactory.Build()
		defer kodo.DeleteDecoder(decoder)
		defer kodo.DeleteDecoder(recoder)

		// Allocate some storage for a "payload" the payload is what we would
		// eventually send over a network
		payload := make([]uint8, encoder.PayloadSize())
		recPayload := make([]uint8, encoder.PayloadSize())

		// Set the storage for the decoder
		dataOut := make([]uint8, len(dataIn))
		decoder.SetMutableSymbols(&dataOut[0], decoder.BlockSize())
		dataRec := make([]uint8, len(dataIn))
		recoder.SetMutableSymbols(&dataRec[0], recoder.BlockSize())

		// Set systematic off
		encoder.SetSystematicOff()

		for !decoder.IsComplete() {
			// Encode the packet into the payload buffer...
			encoder.WritePayload(&payload[0])
			// ...pass that packet to the recoder...
			recoder.ReadPayload(&payload[0])
			// ...recode a packet...
			recoder.WritePayload(&recPayload[0])
			// ...pass that packet to the decoder...
			decoder.ReadPayload(&recPayload[0])
		}

		// Check if we properly decoded the data
		for i, v := range dataIn {
			if v != dataOut[i] {
				fmt.Println("Unexpected failure to decode")
				fmt.Println("Please file a bug report :)")
				return
			}
		}
		fmt.Println("Data decoded correctly")
	}
}
