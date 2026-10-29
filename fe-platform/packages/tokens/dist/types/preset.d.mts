export default preset;
declare namespace preset {
    namespace theme {
        namespace colors {
            let transparent: string;
            let current: string;
        }
        let spacing: {
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
        let borderRadius: {
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
        let fontFamily: {
            sans: string[];
        };
    }
}
