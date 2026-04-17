/// <reference types="vite/client" />

interface Window {
  umami?: {
    track: (event: string, data?: Record<string, unknown>) => void;
  };
}

interface ImportMetaEnv {
  readonly VITE_API_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
