export interface LoginRequest {
  username: string
  password: string
  otp_code?: string
}

export interface LoginResponse {
  token: string
}

export interface TOTPStatus {
  enabled: boolean
}

export interface LauncherConfig {
  name: string
  source_url: string
  mode?: 'release' | 'clone' | 'all'
  include_prerelease?: boolean
  max_versions?: number
}

export interface Config {
  server_port: number
  check_cron: string
  storage_path: string
  download_url_base: string
  admin_user: string
  admin_enabled: boolean
  admin_max_retries: number
  admin_lock_duration: number
  proxy_url: string
  asset_proxy_url: string
  github_token?: string
  concurrent_downloads: number
  download_timeout_minutes: number
  xget_enabled: boolean
  xget_domain: string
  two_factor_enabled: boolean
  two_factor_secret: string
  pow_enabled?: boolean
  pow_algorithm?: string
  pow_cost?: number
  pow_key_length?: number
  pow_difficulty?: number
  pow_challenge_ttl?: string
  download_token_ttl?: string
  traffic_limit_gb?: number
  external_blacklist_url?: string
  appeal_contact?: string
  launchers: LauncherConfig[]
  rate_limit_enabled?: boolean
  rate_limit_per_minute?: number
  rate_limit_ban_threshold?: number
  firewall_whitelist?: string[]
  self_update_enabled?: boolean
  self_update_channel?: string
  self_update_check_cron?: string
  self_update_auto_restart?: boolean
}

export interface ConfigUpdateRequest extends Partial<Config> {
  admin_password?: string
}

export interface TagInfo {
  name: string
  stable: boolean
}

export interface SelfUpdateStatus {
  enabled: boolean
  channel: string
  current_version: string
  latest_version: string
  has_update: boolean
  can_apply: boolean
  pending_restart: boolean
  last_checked_at: string
  last_applied_at: string
  last_check_error: string
  last_apply_error: string
  last_apply_message: string
  available_versions: TagInfo[]
}

export interface FileInfo {
  name: string
  is_dir: boolean
  size: number
  mod_time: string
}

export interface BlacklistItem {
  ip: string
  reason: string
  source: string
  ban_type: string
  created_at: string
}

export interface AddBlacklistRequest {
  ip: string
  reason: string
}

export interface BlacklistStats {
  all: number
  manual?: number
  external?: number
  local?: number
  auto?: number
  [key: string]: number | undefined
}

export interface BlacklistPageData {
  items: BlacklistItem[]
  total: number
  page: number
  page_size: number
  stats: BlacklistStats
}

export interface FirewallStatus {
  settings: {
    enabled: boolean
    per_minute: number
    ban_threshold: number
  }
  whitelist_count: number
  cidr_ban_count: number
  tracked_ips: number
  active_strikes: number
}
