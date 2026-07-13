package middleware

// VIRID deliberately uses a small prime field in the research prototype.  The
// prime is large enough for route-code experiments while keeping a product of
// two field elements inside uint64 without requiring platform-specific 128-bit
// reduction code.
const viridPrime uint64 = 2_147_483_647

type viridPoly []uint64 // coefficients in ascending order: c[0] + c[1]z + ...

func viridFieldAdd(a, b uint64) uint64 {
	s := a + b
	if s >= viridPrime {
		s -= viridPrime
	}
	return s
}

func viridFieldSub(a, b uint64) uint64 {
	if a >= b {
		return a - b
	}
	return viridPrime - (b - a)
}

func viridFieldMul(a, b uint64) uint64 {
	return (a * b) % viridPrime
}

func viridFieldPow(a, exponent uint64) uint64 {
	result := uint64(1)
	for exponent > 0 {
		if exponent&1 == 1 {
			result = viridFieldMul(result, a)
		}
		a = viridFieldMul(a, a)
		exponent >>= 1
	}
	return result
}

func viridFieldInverse(a uint64) uint64 {
	if a == 0 {
		panic("virid: inverse of zero")
	}
	// Fermat's little theorem because viridPrime is prime.
	return viridFieldPow(a, viridPrime-2)
}

func viridPolyNormalize(p viridPoly) viridPoly {
	for len(p) > 0 && p[len(p)-1]%viridPrime == 0 {
		p = p[:len(p)-1]
	}
	if len(p) == 0 {
		return nil
	}
	for i := range p {
		p[i] %= viridPrime
	}
	return p
}

func viridPolyClone(p viridPoly) viridPoly {
	return append(viridPoly(nil), p...)
}

func viridPolyDegree(p viridPoly) int {
	p = viridPolyNormalize(p)
	return len(p) - 1
}

func viridPolyScale(p viridPoly, scalar uint64) viridPoly {
	if scalar%viridPrime == 0 || len(p) == 0 {
		return nil
	}
	out := make(viridPoly, len(p))
	for i, coefficient := range p {
		out[i] = viridFieldMul(coefficient, scalar)
	}
	return viridPolyNormalize(out)
}

func viridPolyMul(a, b viridPoly) viridPoly {
	a = viridPolyNormalize(a)
	b = viridPolyNormalize(b)
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	out := make(viridPoly, len(a)+len(b)-1)
	for i, ac := range a {
		for j, bc := range b {
			out[i+j] = viridFieldAdd(out[i+j], viridFieldMul(ac, bc))
		}
	}
	return viridPolyNormalize(out)
}

// viridPolyMulLinear returns p(z)*(z-root).
func viridPolyMulLinear(p viridPoly, root uint64) viridPoly {
	return viridPolyMul(p, viridPoly{viridFieldSub(0, root%viridPrime), 1})
}

func viridPolyDivMod(numerator, denominator viridPoly) (viridPoly, viridPoly) {
	numerator = viridPolyNormalize(viridPolyClone(numerator))
	denominator = viridPolyNormalize(viridPolyClone(denominator))
	if len(denominator) == 0 {
		panic("virid: polynomial division by zero")
	}
	if len(numerator) < len(denominator) {
		return nil, numerator
	}

	quotient := make(viridPoly, len(numerator)-len(denominator)+1)
	denominatorLeadInverse := viridFieldInverse(denominator[len(denominator)-1])
	for len(numerator) >= len(denominator) && len(numerator) > 0 {
		shift := len(numerator) - len(denominator)
		factor := viridFieldMul(numerator[len(numerator)-1], denominatorLeadInverse)
		quotient[shift] = factor
		for i, coefficient := range denominator {
			index := i + shift
			numerator[index] = viridFieldSub(
				numerator[index],
				viridFieldMul(factor, coefficient),
			)
		}
		numerator = viridPolyNormalize(numerator)
	}
	return viridPolyNormalize(quotient), viridPolyNormalize(numerator)
}

func viridPolyMonic(p viridPoly) viridPoly {
	p = viridPolyNormalize(viridPolyClone(p))
	if len(p) == 0 {
		return nil
	}
	return viridPolyScale(p, viridFieldInverse(p[len(p)-1]))
}

func viridPolyGCD(a, b viridPoly) viridPoly {
	a = viridPolyNormalize(viridPolyClone(a))
	b = viridPolyNormalize(viridPolyClone(b))
	for len(b) > 0 {
		_, remainder := viridPolyDivMod(a, b)
		a, b = b, remainder
	}
	return viridPolyMonic(a)
}

func viridPolyGCDAll(polynomials []viridPoly) viridPoly {
	var result viridPoly
	for _, polynomial := range polynomials {
		polynomial = viridPolyNormalize(polynomial)
		if len(polynomial) == 0 {
			continue
		}
		if len(result) == 0 {
			result = viridPolyMonic(polynomial)
		} else {
			result = viridPolyGCD(result, polynomial)
		}
		if len(result) == 1 { // The unit ideal cannot gain a non-trivial factor later.
			return viridPoly{1}
		}
	}
	return viridPolyMonic(result)
}

func viridPolyEval(p viridPoly, z uint64) uint64 {
	z %= viridPrime
	result := uint64(0)
	for i := len(p) - 1; i >= 0; i-- {
		result = viridFieldAdd(viridFieldMul(result, z), p[i]%viridPrime)
	}
	return result
}
