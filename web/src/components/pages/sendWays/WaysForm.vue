<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Button } from '@/components/ui/button'

import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { CONSTANT } from '@/constant'
import { createValidationState } from '@/util/validation'
// import { validateForm, createValidationState, type InputConfig } from '@/util/validation'
import { toast } from 'vue-sonner'
import { request } from '@/api/api'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import {
  Mail,
  Zap,
  Building2,
  Bird,
  PlugZap,
  Smartphone,
  BellRing,
  Server,
  Check,
  QrCode,
  Send,
  Megaphone,
  ShieldCheck
} from 'lucide-vue-next'

// 组件props
interface Props {
  open?: boolean
  editData?: any // 编辑时传入的数据
  mode?: 'add' | 'edit' // 模式：新增或编辑
}
const props = withDefaults(defineProps<Props>(), {
  open: false,
  editData: null,
  mode: 'add'
})

// 组件emits
const emit = defineEmits<{
  'update:open': [value: boolean]
  'save': [data: any]
}>()

// 前端的页面添加配置
let waysConfigMap = CONSTANT.WAYS_DATA;

// Radio Group 选项 - 根据waysConfigMap动态生成
const channelModeOptions = waysConfigMap.map(item => ({
  value: item.type,
  label: item.label
}))
const channelMode = ref(channelModeOptions[0]?.value || '')

// 当前选中渠道的配置
const currentChannelConfig = computed(() => {
  return waysConfigMap.find(item => item.type === channelMode.value) || null
})

// 表单数据
const formData = ref<Record<string, any>>({})

// 校验状态管理
const validationState = createValidationState()

// 初始化表单数据
const initFormData = () => {
  const config = currentChannelConfig.value
  if (!config) return

  const newFormData: Record<string, any> = {}

  // 如果是编辑模式且有编辑数据，先填充编辑数据
  if (props.mode === 'edit' && props.editData) {
    // 设置渠道类型
    channelMode.value = props.editData.type || channelModeOptions[0]?.value || ''

    // 解析auth数据
    let authData: Record<string, any> = {}
    try {
      authData = props.editData.auth ? JSON.parse(props.editData.auth) : {}
    } catch (e) {
      console.error('解析auth数据失败:', e)
    }

    // 填充基本字段
    newFormData.name = props.editData.name || ''

    // 填充auth中的字段
    Object.keys(authData).forEach(key => {
      newFormData[key] = authData[key]
    })
  }

  // 初始化基本输入字段
  if (config.inputs) {
    config.inputs.forEach((input: any) => {
      if (newFormData[input.col] === undefined) {
        newFormData[input.col] = input.value || ''
      }
    })
  }

  // 初始化任务指令输入字段
  if (config.taskInsInputs) {
    config.taskInsInputs.forEach((input: any) => {
      if (newFormData[input.col] === undefined) {
        newFormData[input.col] = input.value || ''
      }
    })
  }

  // 初始化任务指令单选项
  if (config.taskInsRadios && config.taskInsRadios.length > 0) {
    if (newFormData.taskInsRadio === undefined) {
      newFormData.taskInsRadio = config.taskInsRadios[0].value
    }
  }

  formData.value = newFormData
}

// // 获取所有输入字段配置
// const getAllInputConfigs = (): InputConfig[] => {
//   const config = currentChannelConfig.value
//   if (!config) return []

//   const configs: InputConfig[] = []

//   // 基本输入字段
//   if (config.inputs) {
//     configs.push(...config.inputs.map((input: any) => ({
//       col: input.col,
//       label: input.label,
//       subLabel: input.subLabel,
//       type: input.type,
//       required: input.required !== false,
//       minLength: input.minLength,
//       maxLength: input.maxLength
//     })))
//   }

//   // 任务指令输入字段
//   if (config.taskInsInputs) {
//     configs.push(...config.taskInsInputs.map((input: any) => ({
//       col: input.col,
//       label: input.label,
//       subLabel: input.subLabel,
//       type: input.type,
//       required: input.required !== false,
//       minLength: input.minLength,
//       maxLength: input.maxLength
//     })))
//   }

//   return configs
// }

// // 校验表单
// const validateFormData = () => {
//   const inputConfigs = getAllInputConfigs()
//   const result = validateForm(formData.value, inputConfigs)

//   validationState.setErrors(result.errors)
//   return result.isValid
// }

// 监听渠道模式变化
const handleChannelModeChange = () => {
  initFormData()
  validationState.clearAllErrors()
}

// 监听编辑数据变化（仅编辑模式）
watch(() => props.editData, () => {
  if (props.mode === 'edit') {
    initFormData()
  }
}, { immediate: true })

// 初始化表单数据（新增模式）
if (props.mode === 'add') {
  initFormData()
}

// 关闭drawer
const handleClose = () => {
  emit('update:open', false)
}

// 获取最终提交数据
const getFinalData = () => {
  // 根据当前渠道配置的inputs中的col字段，从formData中提取对应的值组成auth对象
  const config = currentChannelConfig.value
  const authData: Record<string, any> = {}
  if (config && config.inputs) {
    config.inputs.forEach((input: any) => {
      if (formData.value[input.col] !== undefined && input.col != 'name') {
        authData[input.col] = formData.value[input.col]
        if (config.type == 'Email' && input.col == 'port') {
          authData[input.col] = parseInt(formData.value[input.col])
        }
        if (config.type == 'Gotify' && input.col == 'priority') {
          authData[input.col] = parseInt(formData.value[input.col])
        }
      }
    })
  }

  let postData: Record<string, any> = {
    auth: JSON.stringify(authData),
    type: channelMode.value,
    name: formData.value.name,
  }

  // 编辑时需要传递ID
  if (props.mode === 'edit' && props.editData && props.editData.id) {
    postData.id = props.editData.id
  }

  return postData
}

// 测试连接
const handleTest = async () => {
  let postData = getFinalData()
  const rsp = await request.post('/sendways/test', postData)
  if (await rsp.data.code == 200) {
    toast.success(rsp.data.msg)
  }
}

// 保存数据
const handleSave = async () => {
  // if (!validateFormData()) { return }

  let postData = getFinalData()

  // 根据模式选择API路径和成功消息
  const apiUrl = props.mode === 'edit' ? '/sendways/edit' : '/sendways/add'
  const successMessage = props.mode === 'edit' ? '更新渠道成功！' : '添加渠道成功！'

  const rsp = await request.post(apiUrl, postData);
  if (await rsp.data.code == 200) {
    toast(successMessage)
    setTimeout(() => {
      window.location.reload();
    }, 1000);
  }
}

// 渠道图标映射
const getChannelIcon = (type: string) => {
  const map: Record<string, any> = {
    'Email': Mail,
    'Dtalk': Zap,
    'QyWeiXin': Building2,
    'Feishu': Bird,
    'Custom': PlugZap,
    'WeChatOFAccount': QrCode,
    'MessageNest': Server,
    'AliyunSMS': Smartphone,
    'Telegram': Send,
    'Bark': BellRing,
    'Ntfy': Megaphone,
    'Gotify': ShieldCheck,
    'DtalkWork': Zap
  }
  return map[type] || Send // Default icon
}

// 计算保存按钮文本
const saveButtonText = computed(() => {
  return props.mode === 'edit' ? '更新' : '保存'
})
</script>

<template>
  <div class="w-full">
    <!-- Radio Group / 当前渠道展示（编辑模式下只展示当前渠道） -->
    <div class="mb-6">


      <!-- 编辑模式：只展示当前渠道的简洁文本描述，并保留"群发"标识 -->
      <div v-if="props.mode === 'edit'" class="flex items-center gap-1.5 text-sm text-gray-700 dark:text-gray-300">
        <span class="font-medium">{{ currentChannelConfig?.label || channelMode }}</span>
        <span v-if="currentChannelConfig?.dynamicRecipient?.support"
          class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-brand/10 text-brand dark:bg-brand/20 dark:text-brand">群发</span>
      </div>

      <!-- 新增模式：保留原有的单选切换显示 -->
      <!-- 新增模式：卡片式选择 -->
      <!-- 新增模式：水平滚动选择 -->
      <div v-else class="relative group/scroll-container">

        <div class="flex overflow-x-auto gap-3 py-3 px-1 scrollbar-hide -mx-1 select-none">
          <div v-for="option in channelModeOptions" :key="option.value"
            @click="channelMode = option.value; handleChannelModeChange()" :title="option.label"
            class="flex-shrink-0 flex flex-col items-center justify-between p-2.5 h-[95px] min-w-[90px] w-[90px] rounded-lg border cursor-pointer transition-all duration-300 relative overflow-hidden group"
            :class="[
              channelMode === option.value
                ? 'border-primary border-2 bg-primary/5 shadow-md scale-[1.02]'
                : 'border-transparent bg-secondary/30 hover:bg-secondary/60 hover:shadow-sm'
            ]">
            <!-- 选中时的选中标记 -->
            <div v-if="channelMode === option.value"
              class="absolute top-0 right-0 p-0.5 bg-primary rounded-bl-md shadow-sm z-20">
              <Check class="w-2 h-2 text-primary-foreground" stroke-width="3" />
            </div>

            <!-- 图标容器 -->
            <div class="w-10 h-10 rounded-full flex items-center justify-center transition-all duration-300 mt-0.5"
              :class="[
                channelMode === option.value
                  ? 'bg-primary text-primary-foreground shadow-sm'
                  : 'bg-background text-muted-foreground group-hover:text-primary group-hover:scale-110'
              ]">
              <component :is="getChannelIcon(option.value)" class="w-5 h-5" />
            </div>

            <!-- 标签 -->
            <div class="w-full text-center px-0.5">
              <span class="text-[11px] font-medium block truncate"
                :class="channelMode === option.value ? 'text-primary font-bold' : 'text-muted-foreground group-hover:text-foreground'">
                {{ option.label }}
              </span>
            </div>

            <!-- 群发指示点 -->
            <div v-if="waysConfigMap.find(item => item.type === option.value)?.dynamicRecipient?.support"
              class="absolute top-1.5 left-1.5 w-1.5 h-1.5 rounded-full bg-brand ring-1 ring-background z-20"
              title="支持群发">
            </div>
          </div>
        </div>
      </div>

      <!-- 底部简要提示 -->
      <div v-if="props.mode !== 'edit'"
        class="flex items-center justify-end -mt-0.5 gap-2.5 text-[10px] text-muted-foreground px-1">
        <div class="flex items-center gap-1 bg-secondary/50 px-1.5 py-0.5 rounded-full">
          <span class="w-1.5 h-1.5 rounded-full bg-brand ring-1 ring-brand/20"></span>
          <span>支持群发</span>
        </div>
        <div class="flex items-center gap-1 opacity-60">
          <span class="i-lucide-arrow-left-right w-3 h-3"></span>
          <span>左右滑动选择</span>
        </div>
      </div>
    </div>

    <div class="w-full">
      <!-- 动态表单 -->
      <div v-if="currentChannelConfig" class="mt-6">
        <!-- 动态接收者支持提示 -->
        <div v-if="currentChannelConfig.dynamicRecipient?.support"
          class="mb-4 p-2.5 bg-brand/10 dark:bg-brand/5 border border-brand/20 rounded-md">
          <div class="flex items-start gap-2">
            <span class="text-brand text-sm mt-0.5">📧</span>
            <div class="flex-1 space-y-1">
              <p class="text-xs text-brand font-medium">
                支持群发模式 - 可在配置实例时启用"动态接收者"，通过 API 的 <code
                  class="px-1 py-0.5 bg-brand/10 dark:bg-brand/20 rounded text-[11px]">recipients</code> 参数指定多个{{
                    currentChannelConfig.dynamicRecipient.label }}
              </p>
              <p class="text-[11px] text-brand/80">
                适用：邮件群发、公众号批量推送、营销通知等
              </p>
            </div>
          </div>
        </div>

        <!-- 基本配置输入字段 -->
        <div v-if="currentChannelConfig.inputs && currentChannelConfig.inputs.length > 0" class="mb-8">
          <h4 class="text-base font-medium mb-4 text-gray-800">基本配置</h4>
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div v-for="input in currentChannelConfig.inputs" :key="input.col" class="space-y-2" :class="{
              'md:col-span-2': input.isTextArea
            }">
              <Label :for="input.col" class="text-sm font-medium">
                {{ input.subLabel || input.label }}
                <span v-if="input.tips" class="text-xs text-gray-500 ml-1">({{ input.tips }})</span>
              </Label>
              <!-- 配置输入框 -->
              <Textarea v-if="input.isTextArea" :id="input.col" v-model="formData[input.col]"
                :placeholder="input.desc || input.placeholder || input.subLabel || input.label" :class="{
                  'w-full': true,
                  'border-red-500 focus:border-red-500': validationState.errors.value[input.col]
                }" @input="() => validationState.clearFieldError(input.col)" />
              <Input v-else :id="input.col" v-model="formData[input.col]"
                :placeholder="input.desc || input.placeholder || input.subLabel || input.label" :class="{
                  'w-full': true,
                  'border-red-500 focus:border-red-500': validationState.errors.value[input.col]
                }" @input="() => validationState.clearFieldError(input.col)" />
              <!-- 异常校验提示显示 -->
              <div v-if="validationState.errors.value[input.col]" class="text-red-500 text-xs mt-1">
                {{ validationState.errors.value[input.col] }}
              </div>
            </div>
          </div>

          <div class="mt-2 ml-4" v-if="currentChannelConfig.tips && currentChannelConfig.tips.text">
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger class="text-sm hover:text-gray-700 inline-flex items-center gap-1">
                  {{ currentChannelConfig.tips.text }}
                  <span
                    class="cursor-help inline-flex items-center justify-center w-4 h-4 rounded-full border border-gray-300 hover:border-gray-400 text-xs">?</span>
                </TooltipTrigger>
                <TooltipContent class="max-w-md">
                  <div class="text-sm" v-html="currentChannelConfig.tips.desc"></div>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </div>
        </div>
      </div>
      <div v-else class="mt-6 p-6 bg-gray-50 rounded-lg">
        <p class="text-gray-500">请选择一个渠道类型开始配置</p>
      </div>
    </div>

    <div class="flex justify-end gap-2 mt-8 pt-4 border-t">
      <Button variant="outline" @click="handleClose">取消</Button>
      <Button @click="handleTest">测试</Button>
      <Button @click="handleSave">{{ saveButtonText }}</Button>
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent } from 'vue'

export default defineComponent({
  name: 'WaysForm'
})
</script>