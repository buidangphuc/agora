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
        "danger-darker": string;
        "danger-light": string;
        scrim: string;
        promo: string;
        "promo-dark": string;
        success: string;
        "success-dark": string;
    };
    let space: {
        0: number;
        1: number;
        2: number;
        3: number;
        4: number;
        5: number;
        6: number;
        8: number;
        10: number;
        12: number;
        16: number;
        18: number;
    };
    let radius: {
        none: number;
        xs: number;
        lg: number;
        xl: number;
        "2xl": number;
        "3xl": number;
        full: number;
    };
    let fontSize: {
        xs: number[];
        sm: number[];
        base: number[];
        xl: number[];
        "2xl": number[];
        "3xl": number[];
        "4xl": number[];
    };
    namespace fontFamily {
        let sans: string[];
    }
    namespace breakpoint {
        let sm: number;
        let md: number;
        let lg: number;
    }
    let touchTarget: number;
}
export namespace shadow {
    namespace light {
        let card: {
            x: number;
            y: number;
            blur: number;
            spread: number;
            color: string;
        }[];
        let pop: {
            x: number;
            y: number;
            blur: number;
            spread: number;
            color: string;
        }[];
    }
    namespace dark {
        let card_1: {
            x: number;
            y: number;
            blur: number;
            spread: number;
            color: string;
        }[];
        export { card_1 as card };
        let pop_1: {
            x: number;
            y: number;
            blur: number;
            spread: number;
            color: string;
        }[];
        export { pop_1 as pop };
    }
}
export namespace semantic {
    let light_1: {
        "action-primary": string;
        "action-primary-hover": string;
        "action-danger": string;
        "action-danger-hover": string;
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
    export { light_1 as light };
    let dark_1: {
        "action-primary": string;
        "action-primary-hover": string;
        "action-danger": string;
        "action-danger-hover": string;
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
    export { dark_1 as dark };
}
export namespace components {
    export namespace button {
        let base: string;
        namespace variant {
            let primary: string;
            let secondary: string;
            let ghost: string;
            let danger: string;
        }
        namespace size {
            export let xs: string;
            let sm_1: string;
            export { sm_1 as sm };
            let md_1: string;
            export { md_1 as md };
            let lg_1: string;
            export { lg_1 as lg };
        }
        let disabled: string;
        let fullWidth: string;
        let fullWidthMobile: string;
        let icon: string;
        let spinner: string;
    }
    export namespace field {
        let label: string;
        let help: string;
        let error: string;
    }
    export namespace input {
        let base_1: string;
        export { base_1 as base };
        export let invalid: string;
        let disabled_1: string;
        export { disabled_1 as disabled };
    }
    export namespace select {
        let trigger: string;
        let content: string;
        let item: string;
    }
    export namespace checkbox {
        let row: string;
        let root: string;
        let checked: string;
    }
    export namespace dialog {
        export let overlay: string;
        let content_1: string;
        export { content_1 as content };
        export let title: string;
        export let description: string;
    }
    export namespace toast {
        let root_1: string;
        export { root_1 as root };
        let title_1: string;
        export { title_1 as title };
        let description_1: string;
        export { description_1 as description };
    }
    export namespace table {
        let root_2: string;
        export { root_2 as root };
        export let caption: string;
        export let th: string;
        export let td: string;
        export let empty: string;
    }
    export namespace link {
        let base_2: string;
        export { base_2 as base };
    }
    export namespace card_2 {
        let base_3: string;
        export { base_3 as base };
        export let padded: string;
        export let hoverable: string;
        export let loading: string;
        export let skeleton: string;
        export let header: string;
        let title_2: string;
        export { title_2 as title };
        let description_2: string;
        export { description_2 as description };
        let content_2: string;
        export { content_2 as content };
        export let footer: string;
    }
    export { card_2 as card };
}
