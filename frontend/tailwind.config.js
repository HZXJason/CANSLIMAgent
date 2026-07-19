/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['ui-sans-serif', 'system-ui', '-apple-system', 'BlinkMacSystemFont', 'SF Pro Text', 'Segoe UI', 'Microsoft YaHei', 'sans-serif']
      },
      colors: {
        canvas: 'var(--canvas)',
        surface: {
          DEFAULT: 'var(--surface)',
          elevated: 'var(--surface-elevated)',
          muted: 'var(--surface-muted)'
        },
        brand: {
          50: 'rgb(var(--brand-50) / <alpha-value>)',
          100: 'rgb(var(--brand-100) / <alpha-value>)',
          200: 'rgb(var(--brand-200) / <alpha-value>)',
          300: 'rgb(var(--brand-300) / <alpha-value>)',
          400: 'rgb(var(--brand-400) / <alpha-value>)',
          500: 'rgb(var(--brand-500) / <alpha-value>)',
          600: 'rgb(var(--brand-600) / <alpha-value>)',
          700: 'rgb(var(--brand-700) / <alpha-value>)',
          800: 'rgb(var(--brand-800) / <alpha-value>)',
          900: 'rgb(var(--brand-900) / <alpha-value>)',
          950: 'rgb(var(--brand-950) / <alpha-value>)',
          DEFAULT: 'rgb(var(--brand-300) / <alpha-value>)',
          strong: 'rgb(var(--brand-500) / <alpha-value>)'
        },
        finance: {
          navy: '#1E3A5F',
          blue: '#2E5FA3',
          bg: '#111827',
          panel: '#172033',
          muted: '#8EA3B8'
        }
      },
      boxShadow: {
        glow: 'var(--shadow-glow)',
        panel: '0 18px 50px rgba(0, 0, 0, 0.22)',
        float: '0 24px 80px rgba(0, 0, 0, 0.42)'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0', transform: 'translateY(4px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        }
      },
      animation: {
        'fade-in': 'fadeIn 180ms cubic-bezier(0.23, 1, 0.32, 1) forwards'
      }
    }
  },
  plugins: []
};
