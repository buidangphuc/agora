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
        let required: string;
    }
    export namespace input {
        let base_1: string;
        export { base_1 as base };
        export let invalid: string;
        let disabled_1: string;
        export { disabled_1 as disabled };
    }
    export namespace select {
        export let trigger: string;
        export let content: string;
        export let item: string;
        export let native: string;
        let icon_1: string;
        export { icon_1 as icon };
    }
    export namespace checkbox {
        let row: string;
        let root: string;
        let checked: string;
        let mixed: string;
    }
    export namespace dialog {
        export let overlay: string;
        let content_1: string;
        export { content_1 as content };
        export namespace size_1 {
            let sm_2: string;
            export { sm_2 as sm };
            let md_2: string;
            export { md_2 as md };
            let lg_2: string;
            export { lg_2 as lg };
            export let xl: string;
        }
        export { size_1 as size };
        export let header: string;
        export let title: string;
        export let description: string;
        export let close: string;
        export let closeIcon: string;
        export let body: string;
        export let footer: string;
    }
    export namespace toast {
        export namespace viewport {
            let top: string;
            let bottom: string;
        }
        let root_1: string;
        export { root_1 as root };
        export namespace tone {
            export let info: string;
            export let success: string;
            export let warning: string;
            let danger_1: string;
            export { danger_1 as danger };
        }
        let body_1: string;
        export { body_1 as body };
        let title_1: string;
        export { title_1 as title };
        let description_1: string;
        export { description_1 as description };
    }
    export namespace alert {
        let root_2: string;
        export { root_2 as root };
        export namespace tone_1 {
            let info_1: string;
            export { info_1 as info };
            let success_1: string;
            export { success_1 as success };
            let warning_1: string;
            export { warning_1 as warning };
            let danger_2: string;
            export { danger_2 as danger };
        }
        export { tone_1 as tone };
        let icon_2: string;
        export { icon_2 as icon };
        export namespace iconTone {
            let info_2: string;
            export { info_2 as info };
            let success_2: string;
            export { success_2 as success };
            let warning_2: string;
            export { warning_2 as warning };
            let danger_3: string;
            export { danger_3 as danger };
        }
        let body_2: string;
        export { body_2 as body };
        let title_2: string;
        export { title_2 as title };
        let description_2: string;
        export { description_2 as description };
        export let message: string;
        export let action: string;
    }
    export namespace table {
        export let wrap: string;
        let root_3: string;
        export { root_3 as root };
        export let caption: string;
        export let th: string;
        export let td: string;
        let row_1: string;
        export { row_1 as row };
        export let empty: string;
        export namespace align {
            let left: string;
            let center: string;
            let right: string;
        }
        export let sticky: string;
        export namespace hideBelow {
            let sm_3: string;
            export { sm_3 as sm };
            let md_3: string;
            export { md_3 as md };
        }
        export let skeleton: string;
        let error_1: string;
        export { error_1 as error };
    }
    export namespace link {
        let base_2: string;
        export { base_2 as base };
        export let subtle: string;
    }
    export namespace card_2 {
        let base_3: string;
        export { base_3 as base };
        export let padded: string;
        export let hoverable: string;
        export let loading: string;
        let skeleton_1: string;
        export { skeleton_1 as skeleton };
        let header_1: string;
        export { header_1 as header };
        let title_3: string;
        export { title_3 as title };
        let description_3: string;
        export { description_3 as description };
        let content_2: string;
        export { content_2 as content };
        let footer_1: string;
        export { footer_1 as footer };
    }
    export { card_2 as card };
}
