"use client";

import { components } from "@fe-platform/tokens";
import * as RadixToast from "@radix-ui/react-toast";
import {
  type ReactNode,
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
} from "react";
import { Button } from "./Button";
import { cx } from "./cx";

const { toast: toastClasses } = components;

export type ToastTone = keyof typeof toastClasses.tone;

export type ToastMessage = {
  title: string;
  description?: string;
  tone?: ToastTone;
  duration?: number;
};

type Item = ToastMessage & { id: number };

type ToastApi = { toast: (message: ToastMessage) => void };

export type ToastProviderProps = {
  children: ReactNode;
  label?: string;
  dismissLabel?: string;
  position?: keyof typeof toastClasses.viewport;
};

const ToastContext = createContext<ToastApi | null>(null);

let nextId = 0;

/** Wrap the app once; call `useToast().toast(...)` from any client component; `danger` toasts are role=alert, the others role=status; `duration: 0` keeps a toast until dismissed. */
export function ToastProvider({
  children,
  label = "Notifications",
  dismissLabel = "Dismiss",
  position = "bottom",
}: ToastProviderProps) {
  const [items, setItems] = useState<Item[]>([]);
  const toast = useCallback((message: ToastMessage) => {
    nextId += 1;
    setItems((prev) => [...prev, { ...message, id: nextId }]);
  }, []);
  const api = useMemo(() => ({ toast }), [toast]);
  return (
    <ToastContext.Provider value={api}>
      <RadixToast.Provider label={label}>
        {children}
        {items.map((item) => {
          const tone = item.tone ?? "info";
          const danger = tone === "danger";
          const Live = danger ? "div" : "output";
          return (
            <RadixToast.Root
              key={item.id}
              duration={
                item.duration === 0 ? Number.POSITIVE_INFINITY : item.duration
              }
              data-tone={tone}
              className={cx(toastClasses.root, toastClasses.tone[tone])}
              onOpenChange={(open) => {
                if (!open)
                  setItems((prev) => prev.filter((i) => i.id !== item.id));
              }}
            >
              <Live
                role={danger ? "alert" : undefined}
                data-radix-toast-announce-exclude=""
                className={toastClasses.body}
              >
                <RadixToast.Title className={toastClasses.title}>
                  {item.title}
                </RadixToast.Title>
                {item.description && (
                  <RadixToast.Description className={toastClasses.description}>
                    {item.description}
                  </RadixToast.Description>
                )}
              </Live>
              <RadixToast.Close asChild>
                <Button variant="ghost" size="sm">
                  {dismissLabel}
                </Button>
              </RadixToast.Close>
            </RadixToast.Root>
          );
        })}
        <RadixToast.Viewport className={toastClasses.viewport[position]} />
      </RadixToast.Provider>
    </ToastContext.Provider>
  );
}

/** Returns `toast(message)` for transient feedback; only valid under ToastProvider. */
export function useToast(): ToastApi {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used inside ToastProvider");
  return ctx;
}
