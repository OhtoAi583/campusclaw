package kb

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
)

// Embedder 把文本映射成定长向量。做成接口是为了后续迭代能换成真实的语义模型，
// 而不改动检索与索引的上层逻辑。
type Embedder interface {
	Dim() int
	Embed(text string) []float32
}

// HashingEmbedder 是本迭代的默认实现：
// 字符 n-gram（Tokenize 产出）经哈希技巧映射到固定维度，带符号累加，最后 L2 归一化。
//
// 它是"词形层面"的相似，不具备真正的语义泛化（见 design.md D3 的已知限制）。
// 优点是：可离线、确定性（同样输入必得同样向量）、无密钥、无网络依赖，因此排序可复现、可判定。
type HashingEmbedder struct {
	dim int
}

// NewHashingEmbedder 构造默认嵌入器。
func NewHashingEmbedder(dim int) *HashingEmbedder {
	if dim <= 0 {
		dim = 512
	}
	return &HashingEmbedder{dim: dim}
}

// Dim 返回向量维度。
func (h *HashingEmbedder) Dim() int { return h.dim }

// Embed 计算文本向量；结果已 L2 归一化，因此余弦相似度可直接用点积。
func (h *HashingEmbedder) Embed(text string) []float32 {
	vec := make([]float32, h.dim)
	for _, token := range Tokenize(text) {
		sum := fnv.New64a()
		_, _ = sum.Write([]byte(token))
		value := sum.Sum64()
		bucket := int(value % uint64(h.dim))
		sign := float32(1)
		if value&(1<<63) != 0 {
			sign = -1
		}
		vec[bucket] += sign
	}
	return Normalize(vec)
}

// Normalize 做 L2 归一化；零向量原样返回。
func Normalize(vec []float32) []float32 {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return vec
	}
	norm := float32(math.Sqrt(sum))
	for i := range vec {
		vec[i] /= norm
	}
	return vec
}

// Dot 计算点积。两个向量都已归一化时，点积即余弦相似度。
func Dot(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var sum float64
	for i := 0; i < n; i++ {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

// EncodeVector 把向量编码为定长小端 float32 字节串，便于存入 BLOB。
func EncodeVector(vec []float32) []byte {
	out := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(v))
	}
	return out
}

// DecodeVector 解码向量，并校验长度与维度一致。
func DecodeVector(raw []byte, dim int) ([]float32, error) {
	if len(raw) != dim*4 {
		return nil, fmt.Errorf("向量长度不匹配：期望 %d 字节，实际 %d 字节", dim*4, len(raw))
	}
	vec := make([]float32, dim)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return vec, nil
}
