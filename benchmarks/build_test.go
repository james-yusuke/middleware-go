package benchmarklab

import (
	"testing"
	"time"
)

func BenchmarkBuildRoutes(b *testing.B) {
	sizes := benchmarkSizes(b, "ROUTER_BUILD_SIZES", []int{100, 1_000})
	for _, size := range sizes {
		routes := makeCorpus(size)
		for _, factory := range selectedPureFactories(b) {
			b.Run(factory.name+"/routes="+itoa(size), func(b *testing.B) {
				b.ReportAllocs()
				started := time.Now()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					router := factory.new()
					for _, route := range routes {
						if err := router.add(route); err != nil {
							b.Fatal(err)
						}
					}
				}
				b.StopTimer()
				elapsed := time.Since(started)
				b.ReportMetric(float64(b.N*size)/elapsed.Seconds(), "routes/s")
			})
		}
	}
}
