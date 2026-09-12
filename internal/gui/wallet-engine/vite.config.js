import {defineConfig} from 'vite';
export default defineConfig({build:{outDir:'../web/engine',emptyOutDir:true,lib:{entry:'engine.js',formats:['es'],fileName:()=> 'wallet.js'}}});
