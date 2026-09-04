"use client";

import {
  type ReactNode,
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";

export type ToastType = "success" | "error" | "info";

export interface ToastItem {
  id: string;
  type: ToastType;
  message: string;
  duration?: number;
}

interface ToastContextType {
  showToast: (message: string, type?: ToastType, duration?: number) => void;
  success: (message: string, duration?: number) => void;
  error: (message: string, duration?: number) => void;
  info: (message: string, duration?: number) => void;
}

const ToastContext = createContext<ToastContextType | null>(null);

// Global event-based helper for non-react or outside-tree calls
export const toast = {
  show: (message: string, type: ToastType = "info", duration = 3500) => {
    if (typeof window !== "undefined") {
      window.dispatchEvent(
        new CustomEvent("app:toast", {
          detail: { message, type, duration },
        }),
      );
    }
  },
  success: (message: string, duration = 3500) => {
    toast.show(message, "success", duration);
  },
  error: (message: string, duration = 3500) => {
    toast.show(message, "error", duration);
  },
  info: (message: string, duration = 3500) => {
    toast.show(message, "info", duration);
  },
};

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  const removeToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const showToast = useCallback(
    (message: string, type: ToastType = "info", duration = 3500) => {
      const id = `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
      setToasts((prev) => [...prev, { id, type, message, duration }]);

      if (duration > 0) {
        setTimeout(() => {
          removeToast(id);
        }, duration);
      }
    },
    [removeToast],
  );

  const success = useCallback(
    (message: string, duration = 3500) => {
      showToast(message, "success", duration);
    },
    [showToast],
  );

  const error = useCallback(
    (message: string, duration = 3500) => {
      showToast(message, "error", duration);
    },
    [showToast],
  );

  const info = useCallback(
    (message: string, duration = 3500) => {
      showToast(message, "info", duration);
    },
    [showToast],
  );

  // Listen to custom window events
  useEffect(() => {
    function handleCustomToast(event: Event) {
      const customEvt = event as CustomEvent<{
        message: string;
        type?: ToastType;
        duration?: number;
      }>;
      if (customEvt.detail?.message) {
        showToast(
          customEvt.detail.message,
          customEvt.detail.type || "info",
          customEvt.detail.duration || 3500,
        );
      }
    }

    window.addEventListener("app:toast", handleCustomToast);
    return () => window.removeEventListener("app:toast", handleCustomToast);
  }, [showToast]);

  return (
    <ToastContext.Provider value={{ showToast, success, error, info }}>
      {children}

      {/* Floating Toast Notification Container */}
      <div
        aria-live="polite"
        className="pointer-events-none fixed top-4 right-4 z-50 flex max-w-sm w-full flex-col gap-2.5 px-4 sm:px-0"
      >
        {toasts.map((t) => (
          <div
            key={t.id}
            className={`pointer-events-auto flex items-center justify-between gap-3 rounded-xl border p-3.5 shadow-lg backdrop-blur-md transition-all duration-300 ${
              t.type === "success"
                ? "border-emerald-500/30 bg-emerald-900/90 text-white shadow-emerald-950/20"
                : t.type === "error"
                  ? "border-red-500/30 bg-red-900/90 text-white shadow-red-950/20"
                  : "border-gray-700/40 bg-gray-900/90 text-white shadow-black/20"
            }`}
          >
            <div className="flex items-center gap-2.5 min-w-0">
              <span className="shrink-0 text-base">
                {t.type === "success" && "✅"}
                {t.type === "error" && "❌"}
                {t.type === "info" && "ℹ️"}
              </span>
              <p className="text-xs font-medium leading-snug break-words">
                {t.message}
              </p>
            </div>

            <button
              type="button"
              onClick={() => removeToast(t.id)}
              className="shrink-0 rounded-lg p-1 text-white/70 hover:bg-white/20 hover:text-white transition text-xs"
              aria-label="Đóng thông báo"
            >
              ✕
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    // Fallback to global toast methods if called outside provider
    return {
      showToast: toast.show,
      success: toast.success,
      error: toast.error,
      info: toast.info,
    };
  }
  return ctx;
}
