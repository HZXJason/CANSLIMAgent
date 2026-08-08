/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['ui-sans-serif', 'system-ui', '-apple-system', 'BlinkMacSystemFont', 'SF Pro Text', 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', 'sans-serif'],
        display: ['Songti SC', 'Noto Serif SC', 'STSong', 'Times New Roman', 'Georgia', 'serif'],
        data: ['ui-monospace', 'SF Mono', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace']
      },
      colors: {
        canvas: 'var(--canvas)',
        slate: {
          50: '#f7f5f0',
          100: '#eae6dd',
          200: '#d8d1c3',
          300: '#bfb5a2',
          400: '#9d9282',
          500: '#7d7261',
          600: '#5f564a',
          700: '#4a443b',
          800: '#363128',
          900: '#26211b',
          950: '#171310'
        },
        surface: {
          DEFAULT: 'var(--surface)',
          elevated: 'var(--surface-elevated)',
          muted: 'var(--surface-muted)'
        },
        ink: {
          DEFAULT: 'var(--text-primary)',
          secondary: 'var(--text-secondary)',
          tertiary: 'var(--text-tertiary)'
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
        accent: {
          DEFAULT: 'var(--accent)',
          strong: 'var(--accent-strong)'
        },
        'paper-ink': '#171410',
        'paper-light': '#f7f3ea',
        red: {
          50: '#fdf1ec',
          100: '#f9dccf',
          200: '#f3bdaa',
          300: '#eb9579',
          400: '#e2613f',
          500: '#d94f37',
          600: '#c43f2a',
          700: '#a33422',
          800: '#7f2a1d',
          900: '#5e2519',
          950: '#3d1a12'
        },
        emerald: {
          50: '#edf7f1',
          100: '#d8ede2',
          200: '#b8ddca',
          300: '#8fd1ac',
          400: '#5fc08a',
          500: '#4aa976',
          600: '#3c8a60',
          700: '#306e4d',
          800: '#27553c',
          900: '#1e412e',
          950: '#122a1e'
        },
        amber: {
          50: '#fbf4e4',
          100: '#f5e6c3',
          200: '#ecd49a',
          300: '#e2bd6e',
          400: '#d9a441',
          500: '#c98f2e',
          600: '#a97424',
          700: '#855a1d',
          800: '#644417',
          900: '#483211',
          950: '#2e200b'
        },
        positive: 'var(--positive)',
        negative: 'var(--negative)',
        warning: 'var(--warning)',
        rule: 'var(--rule)',
        'rule-strong': 'var(--rule-strong)'
      },
      boxShadow: {
        glow: '0 0 28px rgb(var(--brand-500) / 0.14)',
        lift: 'var(--shadow-lift)',
        panel: '0 18px 50px rgba(0, 0, 0, 0.28)',
        float: '0 24px 80px rgba(0, 0, 0, 0.5)'
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
