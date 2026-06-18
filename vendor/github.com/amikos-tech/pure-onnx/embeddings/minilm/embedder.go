package minilm

import (
	"container/list"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"

	"github.com/amikos-tech/pure-onnx/embeddings/internal/ortutil"
	"github.com/amikos-tech/pure-onnx/ort"
	tokenizers "github.com/amikos-tech/pure-tokenizers"
)

const (
	// DefaultSequenceLength matches the Python all-MiniLM-L6-v2 embedding path.
	DefaultSequenceLength = 256
	// OutputEmbeddingDimension is the all-MiniLM-L6-v2 embedding width.
	OutputEmbeddingDimension = 384
	// DefaultMaxCachedBatchSessions bounds in-memory ONNX session cache growth.
	DefaultMaxCachedBatchSessions = 8

	poolingDenominatorEpsilon = float32(1e-9)
	l2NormEpsilon             = float32(1e-12)
)

const (
	defaultInputIDsName      = "input_ids"
	defaultAttentionMaskName = "attention_mask"
	// #nosec G101 -- ONNX input identifier string, not credential material.
	defaultTokenTypeIDsName = "token_type_ids"
	defaultOutputName       = "last_hidden_state"
)

// PoolingStrategy controls how sequence output is reduced into final embeddings.
type PoolingStrategy string

const (
	PoolingStrategyMean PoolingStrategy = "mean"
	PoolingStrategyCLS  PoolingStrategy = "cls"
	PoolingStrategyNone PoolingStrategy = "none"
)

// Option customizes embedder initialization.
type Option func(*config) error

type config struct {
	sequenceLength       int
	maxCachedBatchCount  int
	tokenizerLibraryPath string
	inputIDsName         string
	attentionMaskName    string
	tokenTypeIDsName     string
	outputName           string
	embeddingDimension   int64
	poolingStrategy      PoolingStrategy
	l2Normalize          bool
	useTokenTypeIDs      bool
}

func defaultConfig() config {
	return config{
		sequenceLength:      DefaultSequenceLength,
		maxCachedBatchCount: DefaultMaxCachedBatchSessions,
		inputIDsName:        defaultInputIDsName,
		attentionMaskName:   defaultAttentionMaskName,
		tokenTypeIDsName:    defaultTokenTypeIDsName,
		outputName:          defaultOutputName,
		embeddingDimension:  OutputEmbeddingDimension,
		poolingStrategy:     PoolingStrategyMean,
		l2Normalize:         true,
		useTokenTypeIDs:     true,
	}
}

// WithSequenceLength sets truncation and fixed padding length.
func WithSequenceLength(length int) Option {
	return func(cfg *config) error {
		if length <= 0 {
			return fmt.Errorf("sequence length must be > 0, got %d", length)
		}
		cfg.sequenceLength = length
		return nil
	}
}

// WithEmbeddingDimension configures the hidden width expected from the model output.
func WithEmbeddingDimension(dim int64) Option {
	return func(cfg *config) error {
		if dim <= 0 {
			return fmt.Errorf("embedding dimension must be > 0, got %d", dim)
		}
		cfg.embeddingDimension = dim
		return nil
	}
}

// WithMeanPooling enables attention-mask-aware mean pooling.
func WithMeanPooling() Option {
	return func(cfg *config) error {
		cfg.poolingStrategy = PoolingStrategyMean
		return nil
	}
}

// WithCLSPooling enables first-token (CLS) pooling.
func WithCLSPooling() Option {
	return func(cfg *config) error {
		cfg.poolingStrategy = PoolingStrategyCLS
		return nil
	}
}

// WithNoPooling keeps token-level output and returns flattened [sequenceLength * embeddingDimension] vectors.
func WithNoPooling() Option {
	return func(cfg *config) error {
		cfg.poolingStrategy = PoolingStrategyNone
		return nil
	}
}

// WithL2Normalization applies L2 normalization to each output embedding row.
func WithL2Normalization() Option {
	return func(cfg *config) error {
		cfg.l2Normalize = true
		return nil
	}
}

// WithoutL2Normalization disables row-level L2 normalization.
func WithoutL2Normalization() Option {
	return func(cfg *config) error {
		cfg.l2Normalize = false
		return nil
	}
}

// WithMaxCachedBatchSessions bounds how many batch-size-specific sessions are cached.
func WithMaxCachedBatchSessions(limit int) Option {
	return func(cfg *config) error {
		if limit <= 0 {
			return fmt.Errorf("max cached batch sessions must be > 0, got %d", limit)
		}
		cfg.maxCachedBatchCount = limit
		return nil
	}
}

// WithTokenizerLibraryPath sets the explicit pure-tokenizers shared library path.
func WithTokenizerLibraryPath(path string) Option {
	return func(cfg *config) error {
		if path == "" {
			return fmt.Errorf("tokenizer library path cannot be empty")
		}
		cfg.tokenizerLibraryPath = path
		return nil
	}
}

// WithoutTokenTypeIDsInput configures the embedder for models that do not consume token_type_ids.
func WithoutTokenTypeIDsInput() Option {
	return func(cfg *config) error {
		cfg.useTokenTypeIDs = false
		cfg.tokenTypeIDsName = ""
		return nil
	}
}

// WithInputOutputNames overrides ONNX input/output names.
// tokenTypeIDsName may be empty for models without token_type_ids.
func WithInputOutputNames(inputIDsName, attentionMaskName, tokenTypeIDsName, outputName string) Option {
	return func(cfg *config) error {
		if inputIDsName == "" || attentionMaskName == "" || outputName == "" {
			return fmt.Errorf("input_ids, attention_mask, and output names cannot be empty")
		}
		cfg.inputIDsName = inputIDsName
		cfg.attentionMaskName = attentionMaskName
		cfg.tokenTypeIDsName = tokenTypeIDsName
		cfg.useTokenTypeIDs = tokenTypeIDsName != ""
		cfg.outputName = outputName
		return nil
	}
}

// Embedder provides local dense transformer embeddings on top of ort.
//
// The default configuration matches all-MiniLM-L6-v2 behavior.
// The caller must initialize ONNX Runtime via ort.SetSharedLibraryPath and
// ort.InitializeEnvironment before calling EmbedDocuments/EmbedQuery.
type Embedder struct {
	modelPath          string
	sequenceLength     int
	embeddingDimension int64
	poolingStrategy    PoolingStrategy
	l2Normalize        bool
	useTokenTypeIDs    bool
	tokenizer          *tokenizers.Tokenizer
	inputNames         []string
	outputNames        []string
	// sessionsByBatch caches one session per unique batch size and is LRU-bounded
	// by maxCachedBatchCount to avoid unbounded memory growth.
	sessionsByBatch     map[int]*embeddingSession
	sessionLRU          *list.List
	sessionLRUIndex     map[int]*list.Element
	maxCachedBatchCount int
	runMu               sync.Mutex
}

type embeddingSession struct {
	inputIDs      []int64
	attentionMask []int64
	tokenTypeIDs  []int64

	inputIDsTensor      *ort.Tensor[int64]
	attentionMaskTensor *ort.Tensor[int64]
	tokenTypeIDsTensor  *ort.Tensor[int64]
	outputTensor        *ort.Tensor[float32]
	session             *ort.AdvancedSession
}

// NewEmbedder creates a high-level dense embedder.
//
// modelPath must point to the local ONNX model file.
// tokenizerPath must point to the local tokenizer.json file.
func NewEmbedder(modelPath string, tokenizerPath string, opts ...Option) (*Embedder, error) {
	if modelPath == "" {
		return nil, fmt.Errorf("model path cannot be empty")
	}
	if tokenizerPath == "" {
		return nil, fmt.Errorf("tokenizer path cannot be empty")
	}
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model path %q is not usable: %w", modelPath, err)
	}
	if _, err := os.Stat(tokenizerPath); err != nil {
		return nil, fmt.Errorf("tokenizer path %q is not usable: %w", tokenizerPath, err)
	}

	cfg := defaultConfig()
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	switch cfg.poolingStrategy {
	case PoolingStrategyMean, PoolingStrategyCLS, PoolingStrategyNone:
	default:
		return nil, fmt.Errorf("unsupported pooling strategy: %q", cfg.poolingStrategy)
	}

	tokenizerOpts := []tokenizers.TokenizerOption{
		tokenizers.WithTruncation(
			uintptr(cfg.sequenceLength),
			tokenizers.TruncationDirectionRight,
			tokenizers.TruncationStrategyLongestFirst,
		),
		tokenizers.WithPadding(true, tokenizers.PaddingStrategy{
			Tag:       tokenizers.PaddingStrategyFixed,
			FixedSize: uintptr(cfg.sequenceLength),
		}),
	}
	if cfg.tokenizerLibraryPath != "" {
		tokenizerOpts = append(tokenizerOpts, tokenizers.WithLibraryPath(cfg.tokenizerLibraryPath))
	}

	tokenizer, err := tokenizers.FromFile(tokenizerPath, tokenizerOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load tokenizer: %w", err)
	}

	inputNames := []string{cfg.inputIDsName, cfg.attentionMaskName}
	if cfg.useTokenTypeIDs {
		inputNames = append(inputNames, cfg.tokenTypeIDsName)
	}

	return &Embedder{
		modelPath:           modelPath,
		sequenceLength:      cfg.sequenceLength,
		embeddingDimension:  cfg.embeddingDimension,
		poolingStrategy:     cfg.poolingStrategy,
		l2Normalize:         cfg.l2Normalize,
		useTokenTypeIDs:     cfg.useTokenTypeIDs,
		tokenizer:           tokenizer,
		inputNames:          inputNames,
		outputNames:         []string{cfg.outputName},
		sessionsByBatch:     make(map[int]*embeddingSession),
		sessionLRU:          list.New(),
		sessionLRUIndex:     make(map[int]*list.Element),
		maxCachedBatchCount: cfg.maxCachedBatchCount,
	}, nil
}

// Close releases ONNX session resources and tokenizer resources.
func (e *Embedder) Close() error {
	if e == nil {
		return nil
	}

	e.runMu.Lock()
	defer e.runMu.Unlock()

	var err error

	for batchSize, session := range e.sessionsByBatch {
		if destroyErr := session.Destroy(); destroyErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to destroy batch-%d embedding resources: %w", batchSize, destroyErr))
		}
	}
	e.sessionsByBatch = nil
	e.sessionLRU = nil
	e.sessionLRUIndex = nil

	if e.tokenizer != nil {
		if closeErr := e.tokenizer.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		e.tokenizer = nil
	}

	return err
}

// EmbedDocuments embeds input documents into deterministic vectors.
func (e *Embedder) EmbedDocuments(documents []string) (_ [][]float32, err error) {
	if e == nil {
		return nil, fmt.Errorf("embedder is nil")
	}
	if len(documents) == 0 {
		return [][]float32{}, nil
	}

	e.runMu.Lock()
	defer e.runMu.Unlock()

	if e.tokenizer == nil || e.sessionsByBatch == nil {
		return nil, fmt.Errorf("embedder has been closed")
	}
	if !ort.IsInitialized() {
		return nil, fmt.Errorf("ONNX Runtime not initialized: call ort.SetSharedLibraryPath and ort.InitializeEnvironment first")
	}

	session, err := e.sessionForBatchLocked(len(documents))
	if err != nil {
		return nil, err
	}

	if err := e.tokenizeInto(
		documents,
		session.inputIDs,
		session.attentionMask,
		session.tokenTypeIDs,
	); err != nil {
		return nil, err
	}

	if err := session.session.Run(); err != nil {
		return nil, fmt.Errorf("embedding inference failed: %w", err)
	}

	embeddings, err := postProcessDenseOutput(
		session.outputTensor.GetData(),
		session.attentionMask,
		len(documents),
		e.sequenceLength,
		e.embeddingDimension,
		e.poolingStrategy,
		e.l2Normalize,
	)
	if err != nil {
		return nil, err
	}

	return embeddings, nil
}

func (e *Embedder) sessionForBatchLocked(batchSize int) (_ *embeddingSession, err error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("batch size must be > 0, got %d", batchSize)
	}

	if session, ok := e.sessionsByBatch[batchSize]; ok {
		e.touchBatchSizeLocked(batchSize)
		return session, nil
	}
	if e.maxCachedBatchCount > 0 && len(e.sessionsByBatch) >= e.maxCachedBatchCount {
		if err := e.evictLeastRecentlyUsedSessionLocked(); err != nil {
			return nil, err
		}
	}

	session, err := newEmbeddingSession(
		e.modelPath,
		e.inputNames,
		e.outputNames,
		e.sequenceLength,
		batchSize,
		e.embeddingDimension,
		e.useTokenTypeIDs,
	)
	if err != nil {
		return nil, err
	}
	e.sessionsByBatch[batchSize] = session
	e.touchBatchSizeLocked(batchSize)
	return session, nil
}

func (e *Embedder) touchBatchSizeLocked(batchSize int) {
	if existing := e.sessionLRUIndex[batchSize]; existing != nil {
		e.sessionLRU.MoveToBack(existing)
		return
	}
	e.sessionLRUIndex[batchSize] = e.sessionLRU.PushBack(batchSize)
}

func (e *Embedder) evictLeastRecentlyUsedSessionLocked() error {
	if e.sessionLRU == nil {
		return nil
	}
	oldest := e.sessionLRU.Front()
	if oldest == nil {
		return nil
	}
	batchSize, ok := oldest.Value.(int)
	if !ok {
		return fmt.Errorf("invalid cache bookkeeping value: %T", oldest.Value)
	}
	session := e.sessionsByBatch[batchSize]
	delete(e.sessionsByBatch, batchSize)
	delete(e.sessionLRUIndex, batchSize)
	e.sessionLRU.Remove(oldest)
	if session == nil {
		return nil
	}
	if err := session.Destroy(); err != nil {
		return fmt.Errorf("failed to evict batch-%d embedding resources: %w", batchSize, err)
	}
	return nil
}

func newEmbeddingSession(modelPath string, inputNames []string, outputNames []string, sequenceLength int, batchSize int, embeddingDimension int64, useTokenTypeIDs bool) (_ *embeddingSession, err error) {
	totalTokens := batchSize * sequenceLength
	inputIDs := make([]int64, totalTokens)
	attentionMask := make([]int64, totalTokens)

	shape := ort.Shape{int64(batchSize), int64(sequenceLength)}
	inputIDsTensor, err := ort.NewTensor[int64](shape, inputIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to create input_ids tensor: %w", err)
	}
	attentionMaskTensor, err := ort.NewTensor[int64](shape, attentionMask)
	if err != nil {
		cleanupErr := ortutil.DestroyAll(inputIDsTensor)
		if cleanupErr != nil {
			return nil, errors.Join(fmt.Errorf("failed to create attention_mask tensor: %w", err), fmt.Errorf("failed to clean up session tensors: %w", cleanupErr))
		}
		return nil, fmt.Errorf("failed to create attention_mask tensor: %w", err)
	}

	var tokenTypeIDs []int64
	var tokenTypeIDsTensor *ort.Tensor[int64]
	if useTokenTypeIDs {
		tokenTypeIDs = make([]int64, totalTokens)
		tokenTypeIDsTensor, err = ort.NewTensor[int64](shape, tokenTypeIDs)
		if err != nil {
			cleanupErr := ortutil.DestroyAll(attentionMaskTensor, inputIDsTensor)
			if cleanupErr != nil {
				return nil, errors.Join(fmt.Errorf("failed to create token_type_ids tensor: %w", err), fmt.Errorf("failed to clean up session tensors: %w", cleanupErr))
			}
			return nil, fmt.Errorf("failed to create token_type_ids tensor: %w", err)
		}
	}

	outputShape := ort.Shape{int64(batchSize), int64(sequenceLength), embeddingDimension}
	outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		cleanupErr := ortutil.DestroyAll(tokenTypeIDsTensor, attentionMaskTensor, inputIDsTensor)
		if cleanupErr != nil {
			return nil, errors.Join(fmt.Errorf("failed to create output tensor: %w", err), fmt.Errorf("failed to clean up session tensors: %w", cleanupErr))
		}
		return nil, fmt.Errorf("failed to create output tensor: %w", err)
	}

	inputValues := []ort.Value{inputIDsTensor, attentionMaskTensor}
	if tokenTypeIDsTensor != nil {
		inputValues = append(inputValues, tokenTypeIDsTensor)
	}

	session, err := ort.NewAdvancedSession(
		modelPath,
		inputNames,
		outputNames,
		inputValues,
		[]ort.Value{outputTensor},
		nil,
	)
	if err != nil {
		cleanupErr := ortutil.DestroyAll(outputTensor, tokenTypeIDsTensor, attentionMaskTensor, inputIDsTensor)
		if cleanupErr != nil {
			return nil, errors.Join(fmt.Errorf("failed to create embedding session: %w", err), fmt.Errorf("failed to clean up session tensors: %w", cleanupErr))
		}
		return nil, fmt.Errorf("failed to create embedding session: %w", err)
	}

	return &embeddingSession{
		inputIDs:            inputIDs,
		attentionMask:       attentionMask,
		tokenTypeIDs:        tokenTypeIDs,
		inputIDsTensor:      inputIDsTensor,
		attentionMaskTensor: attentionMaskTensor,
		tokenTypeIDsTensor:  tokenTypeIDsTensor,
		outputTensor:        outputTensor,
		session:             session,
	}, nil
}

func (s *embeddingSession) Destroy() error {
	if s == nil {
		return nil
	}

	err := ortutil.DestroyAll(
		s.session,
		s.outputTensor,
		s.tokenTypeIDsTensor,
		s.attentionMaskTensor,
		s.inputIDsTensor,
	)

	s.inputIDs = nil
	s.attentionMask = nil
	s.tokenTypeIDs = nil
	s.session = nil
	s.outputTensor = nil
	s.tokenTypeIDsTensor = nil
	s.attentionMaskTensor = nil
	s.inputIDsTensor = nil
	return err
}

// EmbedQuery embeds a single query string.
func (e *Embedder) EmbedQuery(query string) ([]float32, error) {
	embeddings, err := e.EmbedDocuments([]string{query})
	if err != nil {
		return nil, err
	}
	if len(embeddings) != 1 {
		return nil, fmt.Errorf("unexpected embedding row count: got %d, want 1", len(embeddings))
	}
	return embeddings[0], nil
}

func (e *Embedder) tokenizeInto(documents []string, inputIDs []int64, attentionMask []int64, tokenTypeIDs []int64) error {
	sequenceLength := e.sequenceLength
	batchSize := len(documents)
	totalTokens := batchSize * sequenceLength

	if len(inputIDs) != totalTokens || len(attentionMask) != totalTokens {
		return fmt.Errorf(
			"token buffer length mismatch: got input_ids=%d attention_mask=%d, want %d",
			len(inputIDs),
			len(attentionMask),
			totalTokens,
		)
	}
	if tokenTypeIDs != nil && len(tokenTypeIDs) != totalTokens {
		return fmt.Errorf("token_type_ids buffer length mismatch: got %d, want %d", len(tokenTypeIDs), totalTokens)
	}

	clear(inputIDs)
	clear(attentionMask)
	if tokenTypeIDs != nil {
		clear(tokenTypeIDs)
	}

	for i, document := range documents {
		encoding, err := e.tokenizer.Encode(
			document,
			tokenizers.WithAddSpecialTokens(),
			tokenizers.WithReturnAttentionMask(),
			tokenizers.WithReturnTypeIDs(),
		)
		if err != nil {
			return fmt.Errorf("failed to tokenize document %d: %w", i, err)
		}
		if encoding == nil {
			return fmt.Errorf("failed to tokenize document %d: empty tokenizer result", i)
		}

		rowStart := i * sequenceLength
		rowEnd := rowStart + sequenceLength
		fillUint32AsInt64(inputIDs[rowStart:rowEnd], encoding.IDs)

		if len(encoding.AttentionMask) > 0 {
			fillUint32AsInt64(attentionMask[rowStart:rowEnd], encoding.AttentionMask)
		} else {
			deriveAttentionMask(attentionMask[rowStart:rowEnd], inputIDs[rowStart:rowEnd])
		}

		if tokenTypeIDs != nil && len(encoding.TypeIDs) > 0 {
			fillUint32AsInt64(tokenTypeIDs[rowStart:rowEnd], encoding.TypeIDs)
		}
	}

	return nil
}

func fillUint32AsInt64(dst []int64, src []uint32) {
	if len(dst) == 0 || len(src) == 0 {
		return
	}
	copyCount := len(dst)
	if len(src) < copyCount {
		copyCount = len(src)
	}
	for i := 0; i < copyCount; i++ {
		dst[i] = int64(src[i])
	}
}

func deriveAttentionMask(dst []int64, tokenIDs []int64) {
	for i := range dst {
		if tokenIDs[i] != 0 {
			dst[i] = 1
		}
	}
}

func meanPoolAndNormalize(lastHiddenState []float32, attentionMask []int64, batchSize int, sequenceLength int, embeddingDim int64) ([][]float32, error) {
	return postProcessDenseOutput(lastHiddenState, attentionMask, batchSize, sequenceLength, embeddingDim, PoolingStrategyMean, true)
}

func postProcessDenseOutput(lastHiddenState []float32, attentionMask []int64, batchSize int, sequenceLength int, embeddingDim int64, poolingStrategy PoolingStrategy, l2Normalize bool) ([][]float32, error) {
	dim, err := validateDenseOutput(lastHiddenState, attentionMask, batchSize, sequenceLength, embeddingDim)
	if err != nil {
		return nil, err
	}

	var embeddings [][]float32
	switch poolingStrategy {
	case PoolingStrategyMean:
		embeddings = meanPoolTokenEmbeddings(lastHiddenState, attentionMask, batchSize, sequenceLength, dim)
	case PoolingStrategyCLS:
		embeddings = clsPoolTokenEmbeddings(lastHiddenState, batchSize, sequenceLength, dim)
	case PoolingStrategyNone:
		embeddings = flattenTokenEmbeddings(lastHiddenState, batchSize, sequenceLength, dim)
	default:
		return nil, fmt.Errorf("unsupported pooling strategy: %q", poolingStrategy)
	}

	if l2Normalize {
		l2NormalizeRows(embeddings)
	}

	return embeddings, nil
}

func validateDenseOutput(lastHiddenState []float32, attentionMask []int64, batchSize int, sequenceLength int, embeddingDim int64) (int, error) {
	if batchSize <= 0 {
		return 0, fmt.Errorf("batch size must be > 0, got %d", batchSize)
	}
	if sequenceLength <= 0 {
		return 0, fmt.Errorf("sequence length must be > 0, got %d", sequenceLength)
	}
	if embeddingDim <= 0 {
		return 0, fmt.Errorf("embedding dim must be > 0, got %d", embeddingDim)
	}

	expectedMaskLen := batchSize * sequenceLength
	if len(attentionMask) != expectedMaskLen {
		return 0, fmt.Errorf("attention mask length mismatch: got %d, want %d", len(attentionMask), expectedMaskLen)
	}

	dim := int(embeddingDim)
	expectedHiddenLen := expectedMaskLen * dim
	if len(lastHiddenState) != expectedHiddenLen {
		return 0, fmt.Errorf("last_hidden_state length mismatch: got %d, want %d", len(lastHiddenState), expectedHiddenLen)
	}

	return dim, nil
}

func meanPoolTokenEmbeddings(lastHiddenState []float32, attentionMask []int64, batchSize int, sequenceLength int, dim int) [][]float32 {
	embeddings := make([][]float32, batchSize)
	for row := 0; row < batchSize; row++ {
		embedding := make([]float32, dim)
		rowMaskOffset := row * sequenceLength

		denominator := float32(0)
		for tokenIndex := 0; tokenIndex < sequenceLength; tokenIndex++ {
			mask := attentionMask[rowMaskOffset+tokenIndex]
			if mask == 0 {
				continue
			}
			weight := float32(mask)
			denominator += weight

			hiddenOffset := (rowMaskOffset + tokenIndex) * dim
			for d := 0; d < dim; d++ {
				embedding[d] += lastHiddenState[hiddenOffset+d] * weight
			}
		}

		if denominator < poolingDenominatorEpsilon {
			denominator = poolingDenominatorEpsilon
		}
		invDenominator := float32(1.0) / denominator
		for d := 0; d < dim; d++ {
			embedding[d] *= invDenominator
		}

		embeddings[row] = embedding
	}
	return embeddings
}

func clsPoolTokenEmbeddings(lastHiddenState []float32, batchSize int, sequenceLength int, dim int) [][]float32 {
	embeddings := make([][]float32, batchSize)
	stride := sequenceLength * dim
	for row := 0; row < batchSize; row++ {
		rowStart := row * stride
		embedding := make([]float32, dim)
		copy(embedding, lastHiddenState[rowStart:rowStart+dim])
		embeddings[row] = embedding
	}
	return embeddings
}

func flattenTokenEmbeddings(lastHiddenState []float32, batchSize int, sequenceLength int, dim int) [][]float32 {
	embeddings := make([][]float32, batchSize)
	stride := sequenceLength * dim
	for row := 0; row < batchSize; row++ {
		rowStart := row * stride
		embedding := make([]float32, stride)
		copy(embedding, lastHiddenState[rowStart:rowStart+stride])
		embeddings[row] = embedding
	}
	return embeddings
}

func l2NormalizeRows(embeddings [][]float32) {
	for row := range embeddings {
		normSquared := 0.0
		for _, value := range embeddings[row] {
			normSquared += float64(value * value)
		}
		norm := float32(math.Sqrt(normSquared))
		if norm < l2NormEpsilon {
			norm = l2NormEpsilon
		}
		invNorm := float32(1.0) / norm
		for i := range embeddings[row] {
			embeddings[row][i] *= invNorm
		}
	}
}
