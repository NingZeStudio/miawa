import{c as a}from"./index-C6udNFvB.js";/**
 * @license lucide-vue-next v0.475.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const c=a("Trash2Icon",[["path",{d:"M3 6h18",key:"d0wm0j"}],["path",{d:"M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6",key:"4alrt4"}],["path",{d:"M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2",key:"v07s0e"}],["line",{x1:"10",x2:"10",y1:"11",y2:"17",key:"1uufr5"}],["line",{x1:"14",x2:"14",y1:"11",y2:"17",key:"xtxkd"}]]);function i(t){if(!Number.isFinite(t)||t<=0)return"0 B";const e=1024,r=["B","KB","MB","GB","TB","PB","EB"],o=Math.max(0,Math.min(Math.floor(Math.log(t)/Math.log(e)),r.length-1));return parseFloat((t/Math.pow(e,o)).toFixed(2))+" "+r[o]}function s(){const t="ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";let e="";for(let r=0;r<16;r++)e+=t.charAt(Math.floor(Math.random()*t.length));return e}function h(t){const e=`otpauth://totp/LemwoodMirror:admin?secret=${t}&issuer=LemwoodMirror`;return`https://api.qrserver.com/v1/create-qr-code/?size=150x150&data=${encodeURIComponent(e)}`}export{c as T,s as a,i as f,h as g};
