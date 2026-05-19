/**
 * Adapter presets — data, NOT code branches.
 *
 * Each preset is a saved field_map + pagination + auth shape for a
 * known catalog backend. The wizard offers presets as a dropdown so
 * operators do not have to fill 15 fields by hand for the common
 * case. THG's own hub is the first preset; any new backend (Shopify
 * Storefront REST, WooCommerce v3, BigCommerce Catalog API, …) joins
 * this file without touching the wizard component.
 *
 * Operators can always select "Custom" and write the JSON config
 * themselves — presets are convenience, not a closed set.
 */

import type { SourceType } from '../../services/knowledgeService';

export interface AdapterPreset {
  /** Stable identifier used in the dropdown and saved sources. */
  id: string;
  /** Adapter (Go SourceType) this preset emits. */
  adapter: SourceType;
  /** Operator-visible label in the wizard dropdown. */
  label: string;
  /** Short description shown under the label. */
  description: string;
  /** Default base_url, may be edited by the operator. */
  baseUrl?: string;
  /** A complete connection_config blob, ready to send to /knowledge/sources. */
  buildConfig: (baseUrl: string) => unknown;
}

// Field-map preset for the THG Fulfill public hub.
//
// IMPORTANT: This is NOT a vendor branch in code. The wizard reads
// the preset like any other; the adapter receives the resulting JSON
// blob without knowing which preset produced it. Adding a new preset
// is editing this file (data); shipping a new adapter is editing
// internal/workspace_knowledge/ingestion/* (code) — distinct
// activities, distinct review.
const thgHubPreset: AdapterPreset = {
  id: 'thg_hub_v1',
  adapter: 'rest_json',
  label: 'THG Fulfill — public catalog hub',
  description: 'hub.thgfulfill.com paginated JSON. Fields: id, thgSku, name, category, origin, priceFrom/To, currency, images, updatedAt, status.',
  baseUrl: 'https://hub.thgfulfill.com/api/public/catalog',
  buildConfig: (baseUrl: string) => ({
    base_url: baseUrl,
    extractor_version: 'rest_json/v1',
    request: { timeout_seconds: 30, user_agent: 'THGKnowledgeIngestor/1.0' },
    auth: { type: 'none' },
    pagination: {
      scheme: 'page',
      page_param: 'page',
      limit_param: 'limit',
      limit_value: 100,
      start_page: 1,
      total_pages_path: 'pagination.pages',
      max_pages: 10,
    },
    data_path: 'data',
    field_map: {
      source_id: 'id',
      display_sku: 'thgSku',
      vendor_sku: 'sku',
      name: 'name',
      description: '',
      category: 'category',
      origin: 'origin',
      sizes: 'sizes',
      colors: 'colors',
      tags: '',
      price_min: 'priceFrom',
      price_max: 'priceTo',
      currency: 'currency',
      images: 'images',
      source_url_template: 'https://www.thgfulfill.com/catalog/{id}',
      source_updated_at: 'updatedAt',
    },
    availability: {
      from_field: 'status',
      map: { Active: 'in_stock', Inactive: 'out_of_stock' },
      default: 'unknown',
    },
  }),
};

// Custom REST/JSON — empty starting point. Operators fill every field
// themselves. The same blueprint Shopify/WooCommerce presets will use
// once added — see the placeholder shape below.
const customRestJsonPreset: AdapterPreset = {
  id: 'rest_json_custom',
  adapter: 'rest_json',
  label: 'Custom REST/JSON endpoint',
  description: 'Any HTTP+JSON catalog. You configure the field_map and pagination yourself.',
  baseUrl: '',
  buildConfig: (baseUrl: string) => ({
    base_url: baseUrl,
    extractor_version: 'rest_json/v1',
    request: { timeout_seconds: 30, user_agent: 'THGKnowledgeIngestor/1.0' },
    auth: { type: 'none' },
    pagination: { scheme: 'page', page_param: 'page', limit_param: 'limit', limit_value: 50, start_page: 1, max_pages: 50 },
    data_path: 'data',
    field_map: {
      source_id: 'id',
      display_sku: '',
      vendor_sku: '',
      name: 'name',
      description: 'description',
      category: '',
      origin: '',
      sizes: '',
      colors: '',
      tags: '',
      price_min: 'price',
      price_max: 'price',
      currency: 'currency',
      images: 'images',
      source_url_template: '',
      source_updated_at: 'updated_at',
    },
    availability: { from_field: '', map: {}, default: 'unknown' },
  }),
};

export const ADAPTER_PRESETS: AdapterPreset[] = [
  thgHubPreset,
  customRestJsonPreset,
];

export function findPreset(id: string): AdapterPreset | undefined {
  return ADAPTER_PRESETS.find((p) => p.id === id);
}
