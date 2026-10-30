"use client";

import {
  ToastProvider as PlatformToastProvider,
  useToast as usePlatformToast,
} from "@fe-platform/ui";
import {
  type ReactNode,
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
} from "react";

export type ToastType = "success" | "error" | "info";

interface ToastContextType {
  showToast: (message: string, type?: ToastType, duration?: number) => void;
  success: (message: string, duration?: number) => void;
  error: (message: string, duration?: number) => void;
  info: (message: string, duration?: number) => void;
}

const DEFAULT_DURATION = 3500;

const toneFor = {
  success: "success",
  error: "danger",
  info: "info",
} as const;

const ToastContext = createContext<ToastContextType | null>(null);

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

function ToastBridge({ children }: { children: ReactNode }) {
  const { toast: push } = usePlatformToast();

  const showToast = useCallback(
    (
      message: string,
      type: ToastType = "info",
      duration = DEFAULT_DURATION,
    ) => {
      push({ title: message, tone: toneFor[type], duration });
    },
    [push],
  );

  useEffect(() => {
    function handleCustomToast(event: Event) {
      const { detail } = event as CustomEvent<{
        message: string;
        type?: ToastType;
        duration?: number;
      }>;
      if (detail?.message) {
        showToast(
          detail.message,
          detail.type || "info",
          detail.duration || DEFAULT_DURATION,
        );
      }
    }

    window.addEventListener("app:toast", handleCustomToast);
    return () => window.removeEventListener("app:toast", handleCustomToast);
  }, [showToast]);

  const value = useMemo<ToastContextType>(
    () => ({
      showToast,
      success: (message, duration) => showToast(message, "success", duration),
      error: (message, duration) => showToast(message, "error", duration),
      info: (message, duration) => showToast(message, "info", duration),
    }),
    [showToast],
  );

  return (
    <ToastContext.Provider value={value}>{children}</ToastContext.Provider>
  );
}

export function ToastProvider({ children }: { children: ReactNode }) {
  return (
    <PlatformToastProvider position="top" label="Thông báo" dismissLabel="Đóng">
      <ToastBridge>{children}</ToastBridge>
    </PlatformToastProvider>
  );
}

export function useToast(): ToastContextType {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    return {
      showToast: toast.show,
      success: toast.success,
      error: toast.error,
      info: toast.info,
    };
  }
  return ctx;
}
