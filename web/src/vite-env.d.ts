/// <reference types="vite/client" />

interface Window {
  umami?: {
    track: (event: string, data?: Record<string, unknown>) => void;
  };
}

interface ImportMetaEnv {
  readonly VITE_API_URL?: string
  /** Vylth annotator widget token — dev/staging only, never set in production */
  readonly VITE_ANNOT_TOKEN?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
