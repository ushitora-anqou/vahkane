package runner

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/big"

	vahkanev1 "github.com/ushitora-anqou/vahkane/api/v1"
)

var jobEncoding = "0123456789abcdefghijklmnopqrstuvwxyz"

func makeJobName(diName string, action *vahkanev1.DiscordInteractionAction) string {
	var buf bytes.Buffer
	buf.WriteString(diName)
	buf.WriteByte(0)
	buf.WriteString(action.Name)
	hash := sha256.Sum224(buf.Bytes())

	// set hash to x
	x := big.NewInt(0)
	for _, b := range hash {
		x.Lsh(x, 8)
		x.Or(x, big.NewInt(int64(b)))
	}

	// encode x using jobEncoding map
	y := big.NewInt(int64(len(jobEncoding)))
	var encoded bytes.Buffer
	for x.Cmp(y) > 0 {
		var m big.Int
		x.DivMod(x, y, &m)
		c := jobEncoding[m.Int64()]
		encoded.WriteByte(c)
	}
	encoded.WriteByte(jobEncoding[x.Int64()])

	return fmt.Sprintf("job-%s", encoded.String())
}
