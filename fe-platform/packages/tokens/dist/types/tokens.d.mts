export namespace primitives {
    let color: {
        white: string;
        "primary-50": string;
        "primary-100": string;
        "primary-200": string;
        "primary-300": string;
        "primary-400": string;
        "primary-500": string;
        "primary-600": string;
        "primary-700": string;
        "primary-800": string;
        "primary-900": string;
        "primary-950": string;
        "neutral-50": string;
        "neutral-100": string;
        "neutral-200": string;
        "neutral-300": string;
        "neutral-400": string;
        "neutral-500": string;
        "neutral-600": string;
        "neutral-700": string;
        "neutral-800": string;
        "neutral-900": string;
        danger: string;
        "danger-dark": string;
        "danger-light": string;
        scrim: string;
        promo: string;
        "promo-dark": string;
        success: string;
        "success-dark": string;
    };
    let space: {
        0: string;
        1: string;
        2: string;
        3: string;
        4: string;
        5: string;
        6: string;
        8: string;
        10: string;
        12: string;
        16: string;
        18: string;
    };
    let radius: {
        none: string;
        xs: string;
        lg: string;
        xl: string;
        "2xl": string;
        full: string;
    };
    let fontSize: {
        xs: string[];
        sm: string[];
        base: string[];
        xl: string[];
        "2xl": string[];
        "3xl": string[];
        "4xl": string[];
    };
    namespace fontFamily {
        let sans: string[];
    }
}
export namespace semantic {
    let light: {
        "action-primary": string;
        "action-primary-hover": string;
        "surface-page": string;
        "surface-card": string;
        "surface-muted": string;
        "border-subtle": string;
        "border-strong": string;
        "border-danger": string;
        "surface-overlay": string;
        "text-primary": string;
        "text-secondary": string;
        "text-disabled": string;
        "text-inverse": string;
        "text-link": string;
        "text-link-hover": string;
        "text-danger": string;
        "text-success": string;
        "text-warning": string;
        "focus-ring": string;
    };
    let dark: {
        "action-primary": string;
        "action-primary-hover": string;
        "surface-page": string;
        "surface-card": string;
        "surface-muted": string;
        "border-subtle": string;
        "border-strong": string;
        "border-danger": string;
        "surface-overlay": string;
        "text-primary": string;
        "text-secondary": string;
        "text-disabled": string;
        "text-inverse": string;
        "text-link": string;
        "text-link-hover": string;
        "text-danger": string;
        "text-success": string;
        "text-warning": string;
        "focus-ring": string;
    };
}
export namespace components {
    namespace button {
        let base: string;
        namespace variant {
            let primary: string;
            let secondary: string;
            let ghost: string;
        }
        namespace size {
            let sm: string;
            let md: string;
            let lg: string;
        }
        let disabled: string;
    }
    namespace field {
        let label: string;
        let help: string;
        let error: string;
    }
    namespace input {
        let base_1: string;
        export { base_1 as base };
        export let invalid: string;
        let disabled_1: string;
        export { disabled_1 as disabled };
    }
    namespace select {
        let trigger: string;
        let content: string;
        let item: string;
    }
    namespace checkbox {
        let root: string;
        let checked: string;
    }
    namespace dialog {
        export let overlay: string;
        let content_1: string;
        export { content_1 as content };
        export let title: string;
        export let description: string;
    }
    namespace toast {
        let root_1: string;
        export { root_1 as root };
        let title_1: string;
        export { title_1 as title };
        let description_1: string;
        export { description_1 as description };
    }
    namespace table {
        let root_2: string;
        export { root_2 as root };
        export let caption: string;
        export let th: string;
        export let td: string;
        export let empty: string;
    }
    namespace link {
        let base_2: string;
        export { base_2 as base };
    }
    namespace card {
        let base_3: string;
        export { base_3 as base };
    }
}
