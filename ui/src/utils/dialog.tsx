import type { DialogOptions } from 'naive-ui';
import type { DialogApiInjection } from "naive-ui/es/dialog/src/DialogProvider";
import { ref } from "vue";
import { NInput, NButton } from 'naive-ui';


export async function dialogAskConfirm(dialog: DialogApiInjection, title: string | Pick<DialogOptions, 'title' | 'content' | 'positiveText' | 'negativeText'> = '是否进行此操作？', content = '请确认操作无误') {
  return new Promise<boolean>((resolve) => {
    dialog.warning({
      positiveText: '是的',
      negativeText: '取消',
      ...(typeof title === 'string' ? { title, content } : title),
      onPositiveClick: () => {
        resolve(true);
      },
      onNegativeClick: () => {
        resolve(false);
      }
    });
  });
}


export async function dialogInput(dialog: DialogApiInjection, title = '请输入', text = '在这里进行输入') {
  return new Promise<string | undefined>((resolve) => {
    const inputText = ref('');
    const d = dialog.create({
      title: title,
      content: () => {
        return (
          <div>
            <NInput placeholder={text} v-model:value={inputText.value} />
            <div class="mt-4 text-right">
              <NButton type="primary" onClick={async () => {
                resolve(inputText.value);
                d.destroy();
              }}>确定</NButton>
            </div>
          </div>
        )
      },
      onClose: () => {
        resolve(undefined);
      }
    });
  });
}

export async function dialogError(dialog: DialogApiInjection, title = '提示', text = '出现问题') {
  return new Promise<boolean>((resolve) => {
    dialog.error({
      title: title,
      content: text,
      positiveText: '确定',
      onPositiveClick: () => {
        resolve(true);
      }
    });
  });
}
