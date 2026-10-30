export default preset;
declare namespace preset {
    namespace theme {
        namespace colors {
            let transparent: string;
            let current: string;
        }
        let spacing: {
            [k: string]: string;
        };
        let borderRadius: {
            [k: string]: string;
        };
        let fontSize: {
            [k: string]: string[];
        };
        let fontFamily: {
            sans: string[];
        };
        let screens: {
            [k: string]: string;
        };
        namespace minHeight {
            let touch: string;
        }
        let aspectRatio: {
            auto: string;
            square: string;
            video: string;
            "4/3": string;
            "3/4": string;
            "2/1": string;
        };
        namespace boxShadow {
            let none: string;
        }
    }
    let plugins: (typeof safeArea)[];
}
/**
 * @param {{ addUtilities: (u: Record<string, Record<string, string>>) => void }} api
 */
declare function safeArea({ addUtilities }: {
    addUtilities: (u: Record<string, Record<string, string>>) => void;
}): void;
