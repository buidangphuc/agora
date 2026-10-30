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
        14: number;
        16: number;
        18: number;
        20: number;
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
    export namespace skeleton_1 {
        export let block: string;
        export let lines: string;
        export let line: string;
        export let full: string;
        export let short: string;
        export let round: string;
        export namespace avatar {
            let sm_4: string;
            export { sm_4 as sm };
            let md_4: string;
            export { md_4 as md };
            let lg_3: string;
            export { lg_3 as lg };
        }
        export let image: string;
        let card_2: string;
        export { card_2 as card };
        export let cardMedia: string;
        export let aspect: {
            square: string;
            video: string;
            "4/3": string;
            "3/4": string;
            "2/1": string;
        };
    }
    export { skeleton_1 as skeleton };
    export namespace pageSkeleton {
        let root_4: string;
        export { root_4 as root };
        export let heading: string;
        export let hero: string;
        export let grid: string;
        export let tile: string;
    }
    export namespace progress {
        let root_5: string;
        export { root_5 as root };
        export let track: string;
        export namespace size_2 {
            let sm_5: string;
            export { sm_5 as sm };
            let md_5: string;
            export { md_5 as md };
        }
        export { size_2 as size };
        export let fill: string;
        export namespace status {
            export let normal: string;
            let success_3: string;
            export { success_3 as success };
            let danger_4: string;
            export { danger_4 as danger };
        }
        let info_3: string;
        export { info_3 as info };
    }
    export namespace descriptions {
        let root_6: string;
        export { root_6 as root };
        let header_1: string;
        export { header_1 as header };
        let title_3: string;
        export { title_3 as title };
        export let extra: string;
        export let columns: {
            1: string;
            2: string;
            3: string;
        };
        export let bordered: string;
        let item_1: string;
        export { item_1 as item };
        export let itemBordered: string;
        export let term: string;
        export let detail: string;
        export let loading: string;
        let empty_1: string;
        export { empty_1 as empty };
    }
    export namespace statistic {
        let title_4: string;
        export { title_4 as title };
        export let value: string;
        let danger_5: string;
        export { danger_5 as danger };
        export let prefix: string;
        export let suffix: string;
        export let valueSkeleton: string;
        export let trend: string;
        export let up: string;
        export let down: string;
        export let trendLabel: string;
        export let trendSkeleton: string;
        export let fallback: string;
    }
    export namespace link {
        let base_2: string;
        export { base_2 as base };
        export let subtle: string;
    }
    export namespace tabs {
        export let frame: string;
        export let tab: string;
        export let badge: string;
        export let panel: string;
        export namespace variant_1 {
            namespace underline {
                export let list: string;
                let tab_1: string;
                export { tab_1 as tab };
                export let active: string;
                export let inactive: string;
                export let badgeActive: string;
                export let badgeInactive: string;
            }
            namespace pills {
                let list_1: string;
                export { list_1 as list };
                let tab_2: string;
                export { tab_2 as tab };
                let active_1: string;
                export { active_1 as active };
                let inactive_1: string;
                export { inactive_1 as inactive };
                let badgeActive_1: string;
                export { badgeActive_1 as badgeActive };
                let badgeInactive_1: string;
                export { badgeInactive_1 as badgeInactive };
            }
        }
        export { variant_1 as variant };
    }
    export namespace pagination {
        let list_2: string;
        export { list_2 as list };
        export let cell: string;
        export let idle: string;
        let active_2: string;
        export { active_2 as active };
        let disabled_2: string;
        export { disabled_2 as disabled };
        export let gap: string;
    }
    export namespace breadcrumb {
        let list_3: string;
        export { list_3 as list };
        let item_2: string;
        export { item_2 as item };
        let link_1: string;
        export { link_1 as link };
        export let current: string;
        export let separator: string;
    }
    export namespace stepper {
        let list_4: string;
        export { list_4 as list };
        let item_3: string;
        export { item_3 as item };
        export let last: string;
        export let marker: string;
        export namespace markerStatus {
            export let complete: string;
            let current_1: string;
            export { current_1 as current };
            export let upcoming: string;
            export let failed: string;
        }
        let label_1: string;
        export { label_1 as label };
        let title_5: string;
        export { title_5 as title };
        export namespace titleStatus {
            let complete_1: string;
            export { complete_1 as complete };
            let current_2: string;
            export { current_2 as current };
            let upcoming_1: string;
            export { upcoming_1 as upcoming };
            let failed_1: string;
            export { failed_1 as failed };
        }
        let link_2: string;
        export { link_2 as link };
        export let inert: string;
        export let connector: string;
        export namespace dotStatus {
            let complete_2: string;
            export { complete_2 as complete };
            let current_3: string;
            export { current_3 as current };
            let upcoming_2: string;
            export { upcoming_2 as upcoming };
            let failed_2: string;
            export { failed_2 as failed };
        }
    }
    export namespace timeline {
        let list_5: string;
        export { list_5 as list };
        let item_4: string;
        export { item_4 as item };
        let line_1: string;
        export { line_1 as line };
        export let dotWrap: string;
        export let dot: string;
        export namespace tone_2 {
            export let neutral: string;
            let primary_1: string;
            export { primary_1 as primary };
            let info_4: string;
            export { info_4 as info };
            let success_4: string;
            export { success_4 as success };
            let warning_3: string;
            export { warning_3 as warning };
            let danger_6: string;
            export { danger_6 as danger };
        }
        export { tone_2 as tone };
        let body_3: string;
        export { body_3 as body };
        let title_6: string;
        export { title_6 as title };
        export let titleDefault: string;
        export namespace titleTone {
            let neutral_1: string;
            export { neutral_1 as neutral };
            let primary_2: string;
            export { primary_2 as primary };
            let info_5: string;
            export { info_5 as info };
            let success_5: string;
            export { success_5 as success };
            let warning_4: string;
            export { warning_4 as warning };
            let danger_7: string;
            export { danger_7 as danger };
        }
        export let time: string;
        let description_3: string;
        export { description_3 as description };
    }
    export namespace priceTag {
        let root_7: string;
        export { root_7 as root };
        export let price: string;
        export let symbol: string;
        export let original: string;
        export let discount: string;
        export namespace size_3 {
            export namespace sm_6 {
                let symbol_1: string;
                export { symbol_1 as symbol };
                let price_1: string;
                export { price_1 as price };
                let original_1: string;
                export { original_1 as original };
                let discount_1: string;
                export { discount_1 as discount };
            }
            export { sm_6 as sm };
            export namespace md_6 {
                let symbol_2: string;
                export { symbol_2 as symbol };
                let price_2: string;
                export { price_2 as price };
                let original_2: string;
                export { original_2 as original };
                let discount_2: string;
                export { discount_2 as discount };
            }
            export { md_6 as md };
            export namespace lg_4 {
                let symbol_3: string;
                export { symbol_3 as symbol };
                let price_3: string;
                export { price_3 as price };
                let original_3: string;
                export { original_3 as original };
                let discount_3: string;
                export { discount_3 as discount };
            }
            export { lg_4 as lg };
            export namespace xl_1 {
                let symbol_4: string;
                export { symbol_4 as symbol };
                let price_4: string;
                export { price_4 as price };
                let original_4: string;
                export { original_4 as original };
                let discount_4: string;
                export { discount_4 as discount };
            }
            export { xl_1 as xl };
        }
        export { size_3 as size };
    }
    export namespace image_1 {
        export let box: string;
        export namespace radius_1 {
            let xl_2: string;
            export { xl_2 as xl };
            let lg_5: string;
            export { lg_5 as lg };
            export let none: string;
        }
        export { radius_1 as radius };
        let aspect_1: {
            square: string;
            video: string;
            "4/3": string;
            "3/4": string;
            "2/1": string;
        };
        export { aspect_1 as aspect };
        export let picture: string;
        export namespace fit {
            let cover: string;
            let contain: string;
        }
        export let placeholder: string;
        let fallback_1: string;
        export { fallback_1 as fallback };
        let empty_2: string;
        export { empty_2 as empty };
        let icon_3: string;
        export { icon_3 as icon };
    }
    export { image_1 as image };
    export namespace avatar_1 {
        let base_3: string;
        export { base_3 as base };
        export namespace shape {
            let circle: string;
            let square: string;
        }
        export namespace size_4 {
            let sm_7: string;
            export { sm_7 as sm };
            let md_7: string;
            export { md_7 as md };
            let lg_6: string;
            export { lg_6 as lg };
            let xl_3: string;
            export { xl_3 as xl };
        }
        export { size_4 as size };
        export let initials: string;
    }
    export { avatar_1 as avatar };
    export namespace drawer {
        let overlay_1: string;
        export { overlay_1 as overlay };
        let content_2: string;
        export { content_2 as content };
        export namespace placement {
            let left_1: string;
            export { left_1 as left };
            let right_1: string;
            export { right_1 as right };
        }
        export namespace size_5 {
            let sm_8: string;
            export { sm_8 as sm };
            let md_8: string;
            export { md_8 as md };
            let lg_7: string;
            export { lg_7 as lg };
        }
        export { size_5 as size };
        let header_2: string;
        export { header_2 as header };
        let title_7: string;
        export { title_7 as title };
        let description_4: string;
        export { description_4 as description };
        let close_1: string;
        export { close_1 as close };
        let icon_4: string;
        export { icon_4 as icon };
        let body_4: string;
        export { body_4 as body };
        let footer_1: string;
        export { footer_1 as footer };
    }
    export namespace card_3 {
        let base_4: string;
        export { base_4 as base };
        export let padded: string;
        export let hoverable: string;
        let loading_1: string;
        export { loading_1 as loading };
        let skeleton_2: string;
        export { skeleton_2 as skeleton };
        let header_3: string;
        export { header_3 as header };
        let title_8: string;
        export { title_8 as title };
        let description_5: string;
        export { description_5 as description };
        let content_3: string;
        export { content_3 as content };
        let footer_2: string;
        export { footer_2 as footer };
    }
    export { card_3 as card };
    export namespace tone_3 {
        let neutral_2: string;
        export { neutral_2 as neutral };
        let primary_3: string;
        export { primary_3 as primary };
        let info_6: string;
        export { info_6 as info };
        let success_6: string;
        export { success_6 as success };
        let warning_5: string;
        export { warning_5 as warning };
        let danger_8: string;
        export { danger_8 as danger };
    }
    export { tone_3 as tone };
    export namespace tag {
        let base_5: string;
        export { base_5 as base };
        let close_2: string;
        export { close_2 as close };
    }
    export namespace badge_1 {
        let base_6: string;
        export { base_6 as base };
        export namespace shape_1 {
            let square_1: string;
            export { square_1 as square };
            export let pill: string;
        }
        export { shape_1 as shape };
        export namespace size_6 {
            let sm_9: string;
            export { sm_9 as sm };
            let md_9: string;
            export { md_9 as md };
        }
        export { size_6 as size };
        export namespace solid {
            let neutral_3: string;
            export { neutral_3 as neutral };
            let primary_4: string;
            export { primary_4 as primary };
            let success_7: string;
            export { success_7 as success };
            let warning_6: string;
            export { warning_6 as warning };
            let danger_9: string;
            export { danger_9 as danger };
        }
    }
    export { badge_1 as badge };
    export namespace empty_3 {
        let root_8: string;
        export { root_8 as root };
        let image_2: string;
        export { image_2 as image };
        let description_6: string;
        export { description_6 as description };
        let action_1: string;
        export { action_1 as action };
    }
    export { empty_3 as empty };
    export namespace result {
        let root_9: string;
        export { root_9 as root };
        export let iconRow: string;
        let icon_5: string;
        export { icon_5 as icon };
        let title_9: string;
        export { title_9 as title };
        export let subTitle: string;
        export let details: string;
        let extra_1: string;
        export { extra_1 as extra };
    }
}
