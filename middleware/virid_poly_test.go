package middleware

import (
	"reflect"
	"testing"
)

func TestVIRIDPolynomialGCDRecoversCommonRouteCode(t *testing.T) {
	oneTwo := viridPolyMulLinear(viridPolyMulLinear(viridPoly{1}, 1), 2)
	oneThree := viridPolyMulLinear(viridPolyMulLinear(viridPoly{1}, 1), 3)

	gcd := viridPolyGCD(oneTwo, oneThree)
	want := viridPoly{viridPrime - 1, 1} // z - 1
	if !reflect.DeepEqual(gcd, want) {
		t.Fatalf("gcd=%v, want %v", gcd, want)
	}
	if got := viridPolyEval(gcd, 1); got != 0 {
		t.Fatalf("gcd(1)=%d, want 0", got)
	}
	if got := viridPolyEval(gcd, 2); got == 0 {
		t.Fatal("gcd unexpectedly contains route code 2")
	}
}

func TestVIRIDPolynomialDivision(t *testing.T) {
	polynomial := viridPolyMulLinear(viridPolyMulLinear(viridPoly{1}, 4), 7)
	divisor := viridPoly{viridPrime - 4, 1}
	quotient, remainder := viridPolyDivMod(polynomial, divisor)

	if len(remainder) != 0 {
		t.Fatalf("remainder=%v, want zero", remainder)
	}
	want := viridPoly{viridPrime - 7, 1}
	if !reflect.DeepEqual(quotient, want) {
		t.Fatalf("quotient=%v, want %v", quotient, want)
	}
}

func TestVIRIDFieldInverse(t *testing.T) {
	for _, value := range []uint64{1, 2, 17, 65_537, viridPrime - 1} {
		inverse := viridFieldInverse(value)
		if got := viridFieldMul(value, inverse); got != 1 {
			t.Fatalf("%d * inverse = %d, want 1", value, got)
		}
	}
}
