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

const { toast: toastClasses } = components;

export type ToastMessage = { title: string; description?: string };

type Item = ToastMessage & { id: number };

type ToastApi = { toast: (message: ToastMessage) => void };

const ToastContext = createContext<ToastApi | null>(null);

/** Wrap the app once; call `useToast().toast(...)` from any client component. */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<Item[]>([]);
  const toast = useCallback((message: ToastMessage) => {
    setItems((prev) => [...prev, { ...message, id: Date.now() + prev.length }]);
  }, []);
  const api = useMemo(() => ({ toast }), [toast]);
  return (
    <ToastContext.Provider value={api}>
      <RadixToast.Provider label="Notifications">
        {children}
        {items.map((item) => (
          <RadixToast.Root
            key={item.id}
            className={toastClasses.root}
            onOpenChange={(open) => {
              if (!open)
                setItems((prev) => prev.filter((i) => i.id !== item.id));
            }}
          >
            <RadixToast.Title className={toastClasses.title}>
              {item.title}
            </RadixToast.Title>
            {item.description && (
              <RadixToast.Description className={toastClasses.description}>
                {item.description}
              </RadixToast.Description>
            )}
            <RadixToast.Close asChild>
              <Button variant="ghost" size="sm">
                Dismiss
              </Button>
            </RadixToast.Close>
          </RadixToast.Root>
        ))}
        <RadixToast.Viewport className="fixed bottom-4 right-4 flex w-full max-w-lg flex-col gap-2" />
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
