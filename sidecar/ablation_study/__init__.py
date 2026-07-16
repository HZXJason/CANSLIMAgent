"""Reproducible paired ablation of TradingAgents' downstream architecture."""

from .runner import run_study
from .statistics import analyze_study_records

__all__ = ["analyze_study_records", "run_study"]
