package service

import (
	"crypto/rand"
	"fmt"
	"math/big"
	mrand "math/rand/v2"
	"sync"
	"testing"
	"time"
)

// ==================== Альтернативные алгоритмы генерации ====================

// generateShortCodeBase62FromID — Base62 от автоинкремента (детерминированно, нет коллизий)
// Сложность: O(log_62(n)) ≈ O(1) для 64-bit чисел
func generateShortCodeBase62FromID(id int64) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	if id == 0 {
		return string(charset[0])
	}

	result := make([]byte, 0, 8)
	for id > 0 {
		result = append(result, charset[id%62])
		id /= 62
	}

	// Разворачиваем (младшие разряды были добавлены первыми)
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return string(result)
}

// generateShortCodeRandom — случайная строка base62 (текущая реализация, упрощённая)
func generateShortCodeRandom(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	code := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		code[i] = charset[n.Int64()]
	}
	return string(code), nil
}

// SnowflakeID — распределённый генератор ID (Twitter Snowflake)
// 64-bit: 41 bit timestamp | 10 bit machine_id | 12 bit sequence
type SnowflakeID struct {
	machineID int64
	epoch     int64 // custom epoch (ms since some start)
	mu        sync.Mutex
	sequence  int64
	lastMs    int64
}

// NewSnowflakeID создаёт новый Snowflake генератор
func NewSnowflakeID(machineID int64) *SnowflakeID {
	return &SnowflakeID{
		machineID: machineID & 0x3FF, // 10 bit
		epoch:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli(),
	}
}

// Next генерирует следующий Snowflake ID
// Потокобезопасен через sync.Mutex
func (s *SnowflakeID) Next() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli() - s.epoch

	if now == s.lastMs {
		s.sequence = (s.sequence + 1) & 0xFFF // 12 bit
		if s.sequence == 0 {
			// Sequence overflow — ждём следующий миллисекунд
			for now <= s.lastMs {
				now = time.Now().UnixMilli() - s.epoch
			}
		}
	} else {
		s.sequence = 0
	}

	s.lastMs = now
	return (now << 22) | (s.machineID << 12) | s.sequence
}

// generateShortCodeSnowflake — генерация кода через Snowflake + base62
func generateShortCodeSnowflake(sf *SnowflakeID) string {
	return generateShortCodeBase62FromID(sf.Next())
}

// counterBasedGenerator — генератор с обфускацией (counter + shuffle)
type counterBasedGenerator struct {
	counter int64
	mu      sync.Mutex
}

func newCounterBasedGenerator() *counterBasedGenerator {
	return &counterBasedGenerator{}
}

func (g *counterBasedGenerator) next() string {
	g.mu.Lock()
	g.counter++
	id := g.counter
	g.mu.Unlock()
	// Простая обфускация: XOR с константой + битовый реверс
	obfuscated := uint32(id) ^ 0x2F3A1B4C
	// Битовый реверс
	obfuscated = reverseBits32(obfuscated)
	return generateShortCodeBase62FromID(int64(obfuscated))
}

func reverseBits32(n uint32) uint32 {
	n = (n >> 16) | (n << 16)
	n = ((n & 0xFF00FF00) >> 8) | ((n & 0x00FF00FF) << 8)
	n = ((n & 0xF0F0F0F0) >> 4) | ((n & 0x0F0F0F0F) << 4)
	n = ((n & 0xCCCCCCCC) >> 2) | ((n & 0x33333333) << 2)
	n = ((n & 0xAAAAAAAA) >> 1) | ((n & 0x55555555) << 1)
	return n
}

// ==================== Бенчмарки ====================

func BenchmarkGenerateShortCode_CryptoRand(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := generateShortCodeRandom(6)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGenerateShortCode_Base62FromID(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = generateShortCodeBase62FromID(int64(i + 1))
	}
}

func BenchmarkGenerateShortCode_Snowflake(b *testing.B) {
	sf := NewSnowflakeID(1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = generateShortCodeSnowflake(sf)
	}
}

func BenchmarkGenerateShortCode_Snowflake_Parallel(b *testing.B) {
	sf := NewSnowflakeID(1)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = generateShortCodeSnowflake(sf)
		}
	})
}

func BenchmarkGenerateShortCode_CounterObfuscated(b *testing.B) {
	gen := newCounterBasedGenerator()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gen.next()
	}
}

func BenchmarkGenerateShortCode_CounterObfuscated_Parallel(b *testing.B) {
	gen := newCounterBasedGenerator()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = gen.next()
		}
	})
}

// BenchmarkGenerateShortCode_MathRand — для сравнения: math/rand (не crypto-safe, но быстрее)
func BenchmarkGenerateShortCode_MathRand(b *testing.B) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rng := mrand.New(mrand.NewPCG(42, 0))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		code := make([]byte, 6)
		for j := 0; j < 6; j++ {
			code[j] = charset[rng.IntN(len(charset))]
		}
		_ = string(code)
	}
}

// ==================== Тесты корректности ====================

func TestGenerateShortCodeBase62FromID(t *testing.T) {
	tests := []struct {
		id       int64
		expected string
	}{
		{0, "a"},
		{1, "b"},
		{61, "9"},
		{62, "ba"},
		{3843, "99"}, // 62*62 - 1 = 3843
	}

	for _, tt := range tests {
		got := generateShortCodeBase62FromID(tt.id)
		if got != tt.expected {
			t.Errorf("generateShortCodeBase62FromID(%d) = %q, want %q", tt.id, got, tt.expected)
		}
	}
}

func TestSnowflakeID_Uniqueness(t *testing.T) {
	sf := NewSnowflakeID(1)
	seen := make(map[int64]bool)

	for i := 0; i < 10000; i++ {
		id := sf.Next()
		if seen[id] {
			t.Fatalf("duplicate Snowflake ID: %d", id)
		}
		seen[id] = true
	}
}

func TestSnowflakeID_Monotonic(t *testing.T) {
	sf := NewSnowflakeID(1)
	var last int64

	for i := 0; i < 10000; i++ {
		id := sf.Next()
		if id <= last {
			t.Fatalf("non-monotonic Snowflake ID: %d <= %d", id, last)
		}
		last = id
	}
}

func TestCounterBasedGenerator_Uniqueness(t *testing.T) {
	gen := newCounterBasedGenerator()
	seen := make(map[string]bool)

	for i := 0; i < 10000; i++ {
		code := gen.next()
		if seen[code] {
			t.Fatalf("duplicate counter code: %s (at i=%d)", code, i)
		}
		seen[code] = true
	}
}

// ==================== Примеры ====================

func Example_base62FromID() {
	fmt.Println(generateShortCodeBase62FromID(12345))
	fmt.Println(generateShortCodeBase62FromID(1000000))
	// Output:
	// dnh
	// emjc
}
