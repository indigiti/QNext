"""QNext deterministic intelligence runtime."""

from .baseline import baseline_manifest, baseline_probabilities, classify_regime
from .client import MarketCoreHistoryClient, timeframe_to_milliseconds
from .domain import Bar, FeatureVector, ModelManifest, Outcome, Prediction
from .features import build_feature_vector
from .learning import (
    CandidateModel,
    EvaluationMetrics,
    LearnedLinearModel,
    LearningDataset,
    LearningExample,
    PromotionGate,
    build_learning_dataset,
    evaluate_directions,
    train_candidate,
)
from .outcomes import evaluate_outcome
from .prediction import predict
from .registry import ModelRegistry
from .shadow import ShadowComparison, ShadowPair, evaluate_shadow, run_shadow
from .store import ImmutableJSONLStore

__all__ = [
    "Bar",
    "FeatureVector",
    "ModelManifest",
    "Outcome",
    "Prediction",
    "MarketCoreHistoryClient",
    "timeframe_to_milliseconds",
    "baseline_manifest",
    "baseline_probabilities",
    "classify_regime",
    "build_feature_vector",
    "evaluate_outcome",
    "predict",
    "ShadowPair",
    "ShadowComparison",
    "run_shadow",
    "evaluate_shadow",
    "ImmutableJSONLStore",
    "LearningExample",
    "LearningDataset",
    "EvaluationMetrics",
    "PromotionGate",
    "CandidateModel",
    "LearnedLinearModel",
    "build_learning_dataset",
    "evaluate_directions",
    "train_candidate",
    "ModelRegistry",
]
