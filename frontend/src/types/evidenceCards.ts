export interface EvidenceCardItem {
  label: string;
  value: string;
  status: 'ok' | 'unknown' | 'warning';
}

export interface EvidenceCard {
  id: string;
  title: string;
  items: EvidenceCardItem[];
}

export interface EvidenceSource {
  id: string;
  label: string;
  data_as_of?: string;
  status: string;
}

export interface EvidenceCounterItem {
  title: string;
  detail: string;
  source_ids: string[];
}

export interface EvidenceCounterThesis {
  bull_anchors: EvidenceCounterItem[];
  bear_falsifiers: EvidenceCounterItem[];
  blind_spots: EvidenceCounterItem[];
}

export interface EvidenceCardsPack {
  symbol: string;
  data_as_of?: string;
  latest_close?: number | null;
  atr_14?: number | null;
  atr_14_percent?: number | null;
  distance_to_52w_high_percent?: number | null;
  cards: EvidenceCard[];
  sources: EvidenceSource[];
  ai_status: string;
  ai_error?: string;
  counter_thesis: EvidenceCounterThesis | null;
}
